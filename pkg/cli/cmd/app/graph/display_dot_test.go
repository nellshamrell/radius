/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package graph

import (
	"strings"
	"testing"

	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_displayDot_EmptyResources(t *testing.T) {
	result := displayDot([]*corerpv20231001preview.ApplicationGraphResource{}, "myapp")
	expected := `digraph "myapp" {
    rankdir=LR;
    node [style=filled, fontname="Helvetica"];

}
`
	require.Equal(t, expected, result)
}

func Test_displayDot_EmptyAppName(t *testing.T) {
	result := displayDot([]*corerpv20231001preview.ApplicationGraphResource{}, "")
	require.Contains(t, result, `digraph "radius"`)
}

func Test_displayDot_SingleRadiusResource(t *testing.T) {
	resources := []*corerpv20231001preview.ApplicationGraphResource{
		{
			ID:                to.Ptr(containerResourceID),
			Name:              to.Ptr("webapp"),
			Type:              to.Ptr("Applications.Core/containers"),
			ProvisioningState: to.Ptr("Succeeded"),
		},
	}

	result := displayDot(resources, "myapp")
	require.Contains(t, result, `digraph "myapp"`)
	require.Contains(t, result, `"webapp" [label="webapp\n(Applications.Core/containers)", shape=box, fillcolor=lightblue]`)
	require.Contains(t, result, "rankdir=LR")
}

func Test_displayDot_TwoResourcesWithConnection(t *testing.T) {
	dirOut := corerpv20231001preview.DirectionOutbound
	dirIn := corerpv20231001preview.DirectionInbound

	resources := []*corerpv20231001preview.ApplicationGraphResource{
		{
			ID:                to.Ptr(containerResourceID),
			Name:              to.Ptr("webapp"),
			Type:              to.Ptr("Applications.Core/containers"),
			ProvisioningState: to.Ptr("Succeeded"),
			Connections: []*corerpv20231001preview.ApplicationGraphConnection{
				{
					ID:        to.Ptr(redisResourceID),
					Direction: &dirOut,
				},
			},
		},
		{
			ID:                to.Ptr(redisResourceID),
			Name:              to.Ptr("redis"),
			Type:              to.Ptr("Applications.Datastores/redisCaches"),
			ProvisioningState: to.Ptr("Succeeded"),
			Connections: []*corerpv20231001preview.ApplicationGraphConnection{
				{
					ID:        to.Ptr(containerResourceID),
					Direction: &dirIn,
				},
			},
		},
	}

	result := displayDot(resources, "myapp")
	require.Contains(t, result, `"webapp" [label="webapp\n(Applications.Core/containers)", shape=box, fillcolor=lightblue]`)
	require.Contains(t, result, `"redis" [label="redis\n(Applications.Datastores/redisCaches)", shape=box, fillcolor=lightblue]`)
	require.Contains(t, result, `"webapp" -> "redis"`)
	// Inbound connections should NOT produce edges
	require.NotContains(t, result, `"redis" -> "webapp"`)
}

func Test_displayDot_NonRadiusResource(t *testing.T) {
	resources := []*corerpv20231001preview.ApplicationGraphResource{
		{
			ID:                to.Ptr("/planes/radius/local/resourceGroups/default/providers/Microsoft.Storage/storageAccounts/mystorage"),
			Name:              to.Ptr("mystorage"),
			Type:              to.Ptr("Microsoft.Storage/storageAccounts"),
			ProvisioningState: to.Ptr("NotDeployed"),
		},
	}

	result := displayDot(resources, "myapp")
	require.Contains(t, result, `shape=ellipse`)
	require.Contains(t, result, `fillcolor=lightyellow`)
	require.Contains(t, result, `"mystorage"`)
}

func Test_displayDot_MixedResources(t *testing.T) {
	dirOut := corerpv20231001preview.DirectionOutbound

	resources := []*corerpv20231001preview.ApplicationGraphResource{
		{
			ID:                to.Ptr(containerResourceID),
			Name:              to.Ptr("webapp"),
			Type:              to.Ptr("Applications.Core/containers"),
			ProvisioningState: to.Ptr("Succeeded"),
			Connections: []*corerpv20231001preview.ApplicationGraphConnection{
				{
					ID:        to.Ptr("/planes/radius/local/resourceGroups/default/providers/Microsoft.Storage/storageAccounts/mystorage"),
					Direction: &dirOut,
				},
			},
		},
		{
			ID:                to.Ptr("/planes/radius/local/resourceGroups/default/providers/Microsoft.Storage/storageAccounts/mystorage"),
			Name:              to.Ptr("mystorage"),
			Type:              to.Ptr("Microsoft.Storage/storageAccounts"),
			ProvisioningState: to.Ptr("NotDeployed"),
		},
	}

	result := displayDot(resources, "myapp")
	// Radius resource uses box
	require.Contains(t, result, `"webapp" [label="webapp\n(Applications.Core/containers)", shape=box, fillcolor=lightblue]`)
	// Non-Radius resource uses ellipse
	require.Contains(t, result, `"mystorage" [label="mystorage\n(Microsoft.Storage/storageAccounts)", shape=ellipse, fillcolor=lightyellow]`)
	// Edge from webapp to mystorage
	require.Contains(t, result, `"webapp" -> "mystorage"`)
}

func Test_displayDot_DeterministicOutput(t *testing.T) {
	dirOut := corerpv20231001preview.DirectionOutbound

	resources := []*corerpv20231001preview.ApplicationGraphResource{
		{
			ID:                to.Ptr(redisResourceID),
			Name:              to.Ptr("redis"),
			Type:              to.Ptr("Applications.Datastores/redisCaches"),
			ProvisioningState: to.Ptr("Succeeded"),
		},
		{
			ID:                to.Ptr(containerResourceID),
			Name:              to.Ptr("webapp"),
			Type:              to.Ptr("Applications.Core/containers"),
			ProvisioningState: to.Ptr("Succeeded"),
			Connections: []*corerpv20231001preview.ApplicationGraphConnection{
				{
					ID:        to.Ptr(redisResourceID),
					Direction: &dirOut,
				},
			},
		},
	}

	// Run multiple times to verify determinism
	first := displayDot(resources, "myapp")
	for i := 0; i < 10; i++ {
		result := displayDot(resources, "myapp")
		require.Equal(t, first, result, "output should be deterministic on iteration %d", i)
	}
}

func Test_displayDot_DeduplicatedEdges(t *testing.T) {
	dirOut := corerpv20231001preview.DirectionOutbound

	resources := []*corerpv20231001preview.ApplicationGraphResource{
		{
			ID:                to.Ptr(containerResourceID),
			Name:              to.Ptr("webapp"),
			Type:              to.Ptr("Applications.Core/containers"),
			ProvisioningState: to.Ptr("Succeeded"),
			Connections: []*corerpv20231001preview.ApplicationGraphConnection{
				{
					ID:        to.Ptr(redisResourceID),
					Direction: &dirOut,
				},
				{
					ID:        to.Ptr(redisResourceID),
					Direction: &dirOut,
				},
			},
		},
	}

	result := displayDot(resources, "myapp")
	// Count occurrences of the edge — should appear exactly once
	count := 0
	for _, line := range splitLines(result) {
		if line == `    "webapp" -> "redis";` {
			count++
		}
	}
	require.Equal(t, 1, count, "duplicate edges should be deduplicated")
}

func Test_escapeDot(t *testing.T) {
	require.Equal(t, `hello`, escapeDot("hello"))
	require.Equal(t, `hello \"world\"`, escapeDot(`hello "world"`))
	require.Equal(t, ``, escapeDot(""))
}

func Test_resourceNameFromID(t *testing.T) {
	require.Equal(t, "webapp", resourceNameFromID(containerResourceID))
	require.Equal(t, "redis", resourceNameFromID(redisResourceID))
	require.Equal(t, "bad-id", resourceNameFromID("bad-id"))
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		lines = append(lines, line)
	}
	return lines
}
