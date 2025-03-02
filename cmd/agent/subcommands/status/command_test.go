// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStatusCommand(t *testing.T) {
	defer os.Unsetenv("DD_AUTOCONFIG_FROM_ENVIRONMENT") // undo os.Setenv by RunE
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"status", "-j"},
		statusCmd,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{}, cliParams.args)
			require.Equal(t, true, cliParams.jsonStatus)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
			require.Equal(t, true, coreParams.ConfigLoadSysProbe)
		})
}

func TestComponentStatusCommand(t *testing.T) {
	defer os.Unsetenv("DD_AUTOCONFIG_FROM_ENVIRONMENT") // undo os.Setenv by RunE
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"status", "component", "abc"},
		componentStatusCmd,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"abc"}, cliParams.args)
			require.Equal(t, false, cliParams.jsonStatus)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
			require.Equal(t, false, coreParams.ConfigLoadSysProbe)
		})
}
