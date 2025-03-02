// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

var (
	elog           debug.Log
	defaultLogFile = "c:\\programdata\\datadog\\logs\\dogstatsd.log"

	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "c:\\programdata\\datadog"

	enabledVals = map[string]bool{"yes": true, "true": true, "1": true,
		"no": false, "false": false, "0": false}
	subServices = map[string]string{"logs_enabled": "logs_enabled",
		"apm_enabled":     "apm_config.enabled",
		"process_enabled": "process_config.enabled"}
)

func init() {
	pd, err := winutil.GetProgramDataDirForProduct("Datadog Dogstatsd")
	if err == nil {
		DefaultConfPath = pd
		defaultLogFile = filepath.Join(pd, "logs", "dogstatsd.log")
	} else {
		winutil.LogEventViewer(ServiceName, 0x8000000F, defaultLogFile)
	}
}

// ServiceName is the name of the service in service control manager
const ServiceName = "dogstatsd"

// EnableLoggingToFile -- set up logging to file

func main() {
	// set the Agent flavor
	flavor.SetFlavor(flavor.Dogstatsd)

	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
	}
	if !isIntSess {
		runService(false)
		return
	}
	defer log.Flush()

	if err = MakeRootCommand().Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	log.Infof("Service control function")

	ctx, cancel := context.WithCancel(context.Background())
	cliParams := &cliParams{}
	err := runDogstatsdFct(
		cliParams,
		DefaultConfPath,
		func(config config.Component) error { return runAgent(ctx, cliParams, config) })

	if err != nil {
		log.Errorf("Failed to start agent %v", err)
		elog.Error(0xc0000008, err.Error())
		errno = 1 // indicates non-successful return from handler.
		stopAgent(cancel)
		changes <- svc.Status{State: svc.Stopped}
		return
	}
	elog.Info(0x40000003, ServiceName)

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop:
				log.Info("Received stop message from service control manager")
				elog.Info(0x4000000b, ServiceName)
				break loop
			case svc.PreShutdown:
				log.Info("Received pre-shutdown message from service control manager")
				elog.Info(0x4000000d, pkgconfig.ServiceName)
				break loop
			case svc.Shutdown:
				log.Info("Received shutdown message from service control manager")
				elog.Info(0x4000000c, ServiceName)
				break loop
			default:
				log.Warnf("unexpected control request #%d", c)
				elog.Warning(0xc0000005, fmt.Sprint(c.Cmd))
			}
		}
	}
	elog.Info(0x40000006, ServiceName)
	log.Infof("Initiating service shutdown")
	changes <- svc.Status{State: svc.StopPending}
	stopAgent(cancel)
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

	elog.Info(0x40000007, ServiceName)
	run := svc.Run

	err = run(ServiceName, &myservice{})
	if err != nil {
		elog.Error(0xc0000008, err.Error())
		return
	}
	elog.Info(0x40000004, ServiceName)
}
