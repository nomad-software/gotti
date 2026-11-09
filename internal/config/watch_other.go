//go:build !linux

package config

import "errors"

// watchNotify is only implemented on Linux; elsewhere Watch polls.
func watchNotify(string, func()) (func(), error) {
	return nil, errors.New("no file notifications on this system")
}
