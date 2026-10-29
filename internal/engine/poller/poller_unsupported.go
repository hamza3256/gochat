//go:build !darwin && !linux

package poller

import "errors"

func New() (Poller, error) {
	return nil, errors.New("poller: unsupported platform")
}
