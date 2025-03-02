// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/internal/flags"
	"github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

var elog debug.Log

// ServiceName specifies the service name used in the operating system.
const ServiceName = "datadog-trace-agent"

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	ctx, cancelFunc := context.WithCancel(context.Background())

	go func() {
		for {
			select {
			case c := <-r:
				switch c.Cmd {
				case svc.Interrogate:
					changes <- c.CurrentStatus
					// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
					time.Sleep(100 * time.Millisecond)
					changes <- c.CurrentStatus
				case svc.Stop, svc.PreShutdown, svc.Shutdown:
					elog.Info(0x40000006, ServiceName)
					changes <- svc.Status{State: svc.StopPending}
					cancelFunc()
					return
				default:
					elog.Warning(0xc000000A, fmt.Sprint(c.Cmd))
				}
			}
		}
	}()
	elog.Info(0x40000003, ServiceName)
	Run(ctx)

	changes <- svc.Status{State: svc.Stopped}
	return
}

func runService(isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(ServiceName)
	} else {
		elog, err = eventlog.Open(ServiceName)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	run := svc.Run
	if isDebug {
		run = debug.Run
	}
	elog.Info(0x40000007, ServiceName)
	err = run(ServiceName, &myservice{})
	if err != nil {
		elog.Error(0xc0000008, err.Error())
		return
	}
	elog.Info(0x40000004, ServiceName)
}

// main is the main application entry point
func main() {
	flag.Parse()

	if !flags.Win.Foreground {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		}
		if !isIntSess {
			runService(false)
			return
		}
		// sigh.  Go doesn't have boolean xor operator.  The options are mutually exclusive,
		// make sure more than one wasn't specified
		optcount := 0
		if flags.Win.StartService {
			optcount++
		}
		if flags.Win.StopService {
			optcount++
		}
		if optcount > 1 {
			fmt.Println("Incompatible options chosen")
			return
		}
		if flags.Win.StartService {
			if err = startService(); err != nil {
				fmt.Printf("Error starting service %v\n", err)
			}
			return
		}
		if flags.Win.StopService {
			if err = stopService(); err != nil {
				fmt.Printf("Error stopping service %v\n", err)
			}
			return
		}
	}

	// prepare go runtime
	runtime.SetMaxProcs()

	// if we are an interactive session, then just invoke the agent on the command line.
	ctx, cancelFunc := context.WithCancel(context.Background())
	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	// Invoke the Agent
	Run(ctx)
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	err = s.Start("is", "manual-started")
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func stopService() error {
	return controlService(svc.Stop, svc.Stopped)
}

func restartService() error {
	var err error
	if err = stopService(); err == nil {
		err = startService()
	}
	return err
}

func controlService(c svc.Cmd, to svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Control(c)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != to {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}

