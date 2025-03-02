// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows
// +build windows

package memory

import (
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

// For testing purpose
var virtualMemory = winutil.VirtualMemory
var swapMemory = winutil.SwapMemory
var pageMemory = winutil.PagefileMemory
var runtimeOS = runtime.GOOS

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	cacheBytes     *pdhutil.PdhSingleInstanceCounterSet
	committedBytes *pdhutil.PdhSingleInstanceCounterSet
	pagedBytes     *pdhutil.PdhSingleInstanceCounterSet
	nonpagedBytes  *pdhutil.PdhSingleInstanceCounterSet
}

const mbSize float64 = 1024 * 1024

// Configure handles initial configuration/initialization of the check
func (c *Check) Configure(data integration.Data, initConfig integration.Data, source string) (err error) {
	if err := c.CommonConfigure(initConfig, data, source); err != nil {
		return err
	}

	return err
}


// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	var val float64

	// counter ("Memory", "Cache Bytes")
	if c.cacheBytes == nil {
		c.cacheBytes, err = pdhutil.GetEnglishSingleInstanceCounter("Memory", "Cache Bytes")
	}
	if c.cacheBytes != nil {
		val, err = c.cacheBytes.GetValue()
	}
	if err == nil {
		sender.Gauge("system.mem.cached", float64(val)/mbSize, "", nil)
	} else {
		c.Warnf("memory.Check: Could not retrieve value for system.mem.cached: %v", err)
	}

	// counter ("Memory", "Committed Bytes")
	if c.committedBytes == nil {
		c.committedBytes, err = pdhutil.GetEnglishSingleInstanceCounter("Memory", "Committed Bytes")
	}
	if c.committedBytes != nil {
		val, err = c.committedBytes.GetValue()
	}
	if err == nil {
		sender.Gauge("system.mem.committed", float64(val)/mbSize, "", nil)
	} else {
		c.Warnf("memory.Check: Could not retrieve value for system.mem.committed: %v", err)
	}

	// counter ("Memory", "Pool Paged Bytes")
	if c.pagedBytes == nil {
		c.pagedBytes, err = pdhutil.GetEnglishSingleInstanceCounter("Memory", "Pool Paged Bytes")
	}
	if c.pagedBytes != nil {
		val, err = c.pagedBytes.GetValue()
	}
	if err == nil {
		sender.Gauge("system.mem.paged", float64(val)/mbSize, "", nil)
	} else {
		c.Warnf("memory.Check: Could not retrieve value for system.mem.paged: %v", err)
	}

	// counter ("Memory", "Pool Nonpaged Bytes")
	if c.nonpagedBytes == nil {
		c.nonpagedBytes, err = pdhutil.GetEnglishSingleInstanceCounter("Memory", "Pool Nonpaged Bytes")
	}
	if c.nonpagedBytes != nil {
		val, err = c.nonpagedBytes.GetValue()
	}
	if err == nil {
		sender.Gauge("system.mem.nonpaged", float64(val)/mbSize, "", nil)
	} else {
		c.Warnf("memory.Check: Could not retrieve value for system.mem.nonpaged: %v", err)
	}

	v, errVirt := virtualMemory()
	if errVirt == nil {
		sender.Gauge("system.mem.total", float64(v.Total)/mbSize, "", nil)
		sender.Gauge("system.mem.free", float64(v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.usable", float64(v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.used", float64(v.Total-v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.pct_usable", float64(100-v.UsedPercent)/100, "", nil)
	} else {
		c.Warnf("memory.Check: could not retrieve virtual memory stats: %s", errVirt)
	}

	s, errSwap := swapMemory()
	if errSwap == nil {
		sender.Gauge("system.swap.total", float64(s.Total)/mbSize, "", nil)
		sender.Gauge("system.swap.free", float64(s.Free)/mbSize, "", nil)
		sender.Gauge("system.swap.used", float64(s.Used)/mbSize, "", nil)
		sender.Gauge("system.swap.pct_free", float64(100-s.UsedPercent)/100, "", nil)
	} else {
		c.Warnf("memory.Check: could not retrieve swap memory stats: %s", errSwap)
	}

	p, errPage := pageMemory()
	if errPage == nil {
		sender.Gauge("system.mem.pagefile.pct_free", float64(100-p.UsedPercent)/100, "", nil)
		sender.Gauge("system.mem.pagefile.total", float64(p.Total)/mbSize, "", nil)
		sender.Gauge("system.mem.pagefile.free", float64(p.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.pagefile.used", float64(p.Used)/mbSize, "", nil)
	} else {
		c.Warnf("memory.Check: could not retrieve swap memory stats: %s", errSwap)
	}

	sender.Commit()
	return nil
}
