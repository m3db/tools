// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package parallel

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var errCmdFailed = errors.New("command failed")

type cmdController struct {
	Cmd          *exec.Cmd
	EventHandler func(*Event)
	Clock        func() time.Time
	Started      bool
	Finished     bool
	StartTime    time.Time
	Lock         sync.Mutex
}

func newCmdController(cmd *exec.Cmd, eventHandler func(*Event), clock func() time.Time) *cmdController {
	return &cmdController{cmd, eventHandler, clock, false, false, clock(), sync.Mutex{}}
}

// Run returns false on failure that has not been already handled
func (c *cmdController) Run() bool {
	c.Lock.Lock()
	if c.Started || c.Finished {
		c.Lock.Unlock()
		return true
	}
	c.Started = true
	c.StartTime = c.Clock()
	c.EventHandler(newCmdStartedEvent(c.StartTime, c.Cmd))
	if err := c.Cmd.Start(); err != nil {
		finishTime := c.Clock()
		err = fmt.Errorf("command could not start: %s: %v", cmdString(c.Cmd), err)
		c.Finished = true
		c.EventHandler(newCmdFinishedEvent(finishTime, c.Cmd, c.StartTime, err))
		c.Lock.Unlock()
		return false
	}
	c.Lock.Unlock()
	err := c.Cmd.Wait()
	finishTime := c.Clock()
	if err != nil {
		err = fmt.Errorf("command had error: %s: %s", cmdString(c.Cmd), err.Error())
	}
	c.Lock.Lock()
	defer c.Lock.Unlock()
	if c.Finished {
		return true
	}
	c.Finished = true
	c.EventHandler(newCmdFinishedEvent(finishTime, c.Cmd, c.StartTime, err))
	return err == nil
}

func (c *cmdController) Kill() {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	if !c.Started {
		c.Started = true
		c.Finished = true
		return
	}
	if c.Finished {
		return
	}
	c.Finished = true
	if c.Cmd.Process != nil {
		err := c.Cmd.Process.Kill()
		finishTime := c.Clock()
		if err != nil {
			err = fmt.Errorf("command had error on kill: %s: %s", cmdString(c.Cmd), err.Error())
		}
		c.EventHandler(newCmdFinishedEvent(finishTime, c.Cmd, c.StartTime, err))
	}
}

func cmdString(cmd *exec.Cmd) string {
	return strings.Join(append([]string{cmd.Path}, cmd.Args...), " ")
}
