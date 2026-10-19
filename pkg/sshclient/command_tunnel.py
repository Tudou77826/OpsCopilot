# Executed with python3 -u -c through an SSH exec channel, without a PTY.
# Stdout is a nonce-bound startup frame followed exclusively by database bytes.
import json
import os
import select
import socket
import sys
import threading
import time

# Linux is the deployment target. Binary CRT mode also permits byte-exact tests
# of this same script on Windows without replacing the relay implementation.
if os.name == "nt":
    import msvcrt
    msvcrt.setmode(0, os.O_BINARY)
    msvcrt.setmode(1, os.O_BINARY)

host, port, timeout, token = sys.argv[1:5]
sock = None
phase = "connect"
closing = threading.Event()
input_ended = threading.Event()
failed = threading.Event()
status_lock = threading.Lock()

def report(error):
    failed.set()
    # No payload, arguments or raw exception text is emitted.
    status = {"phase": phase, "kind": type(error).__name__, "errno": getattr(error, "errno", None)}
    with status_lock:
        try:
            sys.stderr.write("OC_TCP_ERROR " + token + " " + json.dumps(status) + "\n")
            sys.stderr.flush()
        except OSError:
            pass

def send_input():
    try:
        while not closing.is_set():
            chunk = os.read(0, 65536)
            if not chunk:
                input_ended.set()
                sock.shutdown(socket.SHUT_WR)
                return
            sock.sendall(chunk)
    except Exception as error:
        if not closing.is_set():
            report(error)
            try:
                sock.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass

try:
    sock = socket.create_connection((host, int(port)), float(timeout))
    sys.stdout.buffer.write(("OC_TCP_READY " + token + "\n").encode("ascii"))
    sys.stdout.buffer.flush()
    phase = "relay"
    sock.settimeout(None)
    sender = threading.Thread(target=send_input)
    sender.daemon = True
    sender.start()
    drain_until = None
    while not failed.is_set():
        # Bound orphan lifetime after SSH stdin disappears. Active SQL has no
        # relay idle timeout. After a half-close, keep draining peer responses.
        if input_ended.is_set() and drain_until is None:
            drain_until = time.monotonic() + 10
        if drain_until is not None and time.monotonic() >= drain_until:
            break
        readable, _, _ = select.select([sock], [], [], 0.5)
        if not readable:
            continue
        chunk = sock.recv(65536)
        if not chunk:
            # Signal peer read EOF while allowing client writes to finish.
            # An abandoned half-close expires in 10s.
            os.close(1)
            # SSH EOF covers both stdout and stderr. Close both output streams
            # so the SSH peer can observe read EOF while stdin is still usable.
            os.close(2)
            sender.join(10)
            break
        if drain_until is not None:
            drain_until = time.monotonic() + 10
        view = memoryview(chunk)
        while view:
            count = os.write(1, view)
            view = view[count:]
except Exception as error:
    report(error)
finally:
    closing.set()
    if sock is not None:
        sock.close()
if failed.is_set():
    sys.exit(1)
