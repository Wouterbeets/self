//go:build !linux

package main

import (
	"fmt"
	"time"
)

func (p *playpen) exec(command string, timeout time.Duration) ([]byte, error) {
	return nil, fmt.Errorf("playpen jail is only available on Linux")
}

func cmdJail(root, command string) error {
	return fmt.Errorf("playpen jail is only available on Linux")
}
