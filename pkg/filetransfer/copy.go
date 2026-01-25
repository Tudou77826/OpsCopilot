package filetransfer

import (
	"context"
	"io"
	"time"
)

func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, total int64, progress func(Progress)) (int64, error) {
	buf := make([]byte, 64*1024)
	var done int64

	lastReport := time.Now()
	lastBytes := int64(0)

	report := func(now time.Time) {
		if progress == nil {
			return
		}
		elapsed := now.Sub(lastReport).Seconds()
		speed := int64(0)
		if elapsed > 0 {
			speed = int64(float64(done-lastBytes) / elapsed)
		}
		progress(Progress{BytesDone: done, BytesTotal: total, SpeedBps: speed})
		lastReport = now
		lastBytes = done
	}

	for {
		select {
		case <-ctx.Done():
			return done, &TransferError{Code: ErrorCodeUnknown, Message: "传输已取消"}
		default:
		}

		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				done += int64(nw)
			}
			if ew != nil {
				return done, toTransferError(ew)
			}
			if nr != nw {
				return done, &TransferError{Code: ErrorCodeUnknown, Message: "写入不完整"}
			}
		}
		now := time.Now()
		if now.Sub(lastReport) >= 200*time.Millisecond {
			report(now)
		}
		if er != nil {
			if er == io.EOF {
				report(time.Now())
				return done, nil
			}
			return done, toTransferError(er)
		}
	}
}
