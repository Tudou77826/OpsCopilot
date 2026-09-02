package shellsidecar

// 手工探针：对运行中的 sidecar --dev 走浏览器同款序列（控制面 + 数据面）。
// 运行：PROBE=1 go test ./internal/shellsidecar -run ProbeDevFlow -v

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestProbeDevFlow(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("手工探针：PROBE=1 时运行")
	}
	ws, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:9777/rpc?token=devtoken", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	type frame struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	responses := make(chan frame, 16)
	go func() {
		for {
			var f frame
			if err := ws.ReadJSON(&f); err != nil {
				return
			}
			responses <- f
		}
	}()
	call := func(id int, method string, params any) (frame, error) {
		payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		fmt.Printf("-> %s\n", method)
		if err := ws.WriteJSON(json.RawMessage(payload)); err != nil {
			return frame{}, err
		}
		timeout := time.After(8 * time.Second)
		for {
			select {
			case f := <-responses:
				if f.Method != "" {
					fmt.Printf("<- notify %s %s\n", f.Method, string(f.Result))
					continue
				}
				var gotID int
				_ = json.Unmarshal(f.ID, &gotID)
				if gotID != id {
					continue
				}
				if f.Error != nil {
					return f, fmt.Errorf("rpc error: %s", f.Error.Message)
				}
				fmt.Printf("<- %s ok: %s\n", method, string(f.Result))
				return f, nil
			case <-timeout:
				return frame{}, fmt.Errorf("TIMEOUT waiting %s", method)
			}
		}
	}

	if _, err := call(1, "initialize", nil); err != nil {
		t.Fatal(err)
	}
	conn, err := call(2, "shell.connect", map[string]any{"config": map[string]any{"host": "127.0.0.1", "port": 59330, "user": "test", "password": "test"}})
	if err != nil {
		t.Fatal(err)
	}
	var cr struct {
		ConnectionID string `json:"connectionId"`
	}
	_ = json.Unmarshal(conn.Result, &cr)
	term, err := call(3, "shell.openTerminal", map[string]any{"connectionId": cr.ConnectionID, "cols": 80, "rows": 24})
	if err != nil {
		t.Fatal(err)
	}
	var tr struct {
		TerminalID string `json:"terminalId"`
	}
	_ = json.Unmarshal(term.Result, &tr)

	// ---- 数据面：挂载 → 收重放/横幅 → 发字节 → 收回显 ----
	dialURL := fmt.Sprintf("ws://127.0.0.1:9777/terminals/%s?token=devtoken", tr.TerminalID)
	dataWS, resp, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("数据面连接失败: %v (http resp: %v)", err, resp)
	}
	defer dataWS.Close()
	dataWS.SetReadDeadline(time.Now().Add(5 * time.Second))
	var replay []byte
	_ = replay
	// 读到第一条消息（横幅或重放）
	_, msg, err := dataWS.ReadMessage()
	if err != nil {
		t.Fatalf("读数据面失败: %v", err)
	}
	replay = msg
	fmt.Printf("<- data first frame: %q\n", string(replay))
	if err := dataWS.WriteMessage(websocket.BinaryMessage, []byte("ping123\r")); err != nil {
		t.Fatal(err)
	}
	_ = dataWS.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := 0; i < 5; i++ {
		_, msg, err = dataWS.ReadMessage()
		if err != nil {
			t.Fatalf("读回显失败: %v", err)
		}
		fmt.Printf("<- data: %q\n", string(msg))
		if len(msg) >= 8 && string(msg[len(msg)-8:]) == "ping123\r" {
			fmt.Println("回显命中，数据面 OK")
			break
		}
	}
	// 清理
	_, _ = call(9, "shell.disconnect", map[string]any{"connectionId": cr.ConnectionID})
}

var _ = http.StatusOK
