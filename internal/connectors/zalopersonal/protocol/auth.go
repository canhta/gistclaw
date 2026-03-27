package protocol

import (
	"context"
	"errors"
)

var ErrQRLoginNotImplemented = errors.New("zalo personal protocol: qr login not implemented")

func LoginQR(ctx context.Context, qrCallback func([]byte)) (*Credentials, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, ErrQRLoginNotImplemented
}
