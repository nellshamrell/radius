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
	"fmt"
	"sort"
	"strings"

	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// displayDot produces a Graphviz DOT language digraph string from the application graph.
// Radius resources are rendered as box nodes with a lightblue fill, non-Radius resources
// as ellipse nodes with a lightyellow fill. Outbound connections are rendered as directed
// edges. The output is deterministic: nodes sorted by type then name, edges deduplicated
// and sorted.
func displayDot(applicationResources []*v20231001preview.ApplicationGraphResource, appName string) string {
	graphName := appName
	if graphName == "" {
		graphName = "radius"
	}

	output := &strings.Builder{}
	output.WriteString(fmt.Sprintf("digraph %q {\n", graphName))
	output.WriteString("    rankdir=LR;\n")
	output.WriteString("    node [style=filled, fontname=\"Helvetica\"];\n\n")

	// Sort resources for deterministic output
	sorted := make([]*v20231001preview.ApplicationGraphResource, len(applicationResources))
	copy(sorted, applicationResources)
	sort.Slice(sorted, func(i, j int) bool {
		if *sorted[i].Type != *sorted[j].Type {
			return *sorted[i].Type < *sorted[j].Type
		}
		return *sorted[i].Name < *sorted[j].Name
	})

	// Emit nodes
	for _, r := range sorted {
		name := escapeDot(*r.Name)
		typeName := escapeDot(*r.Type)
		if isRadiusResource("", *r.Type) {
			output.WriteString(fmt.Sprintf("    %q [label=\"%s\\n(%s)\", shape=box, fillcolor=lightblue];\n",
				name, name, typeName))
		} else {
			output.WriteString(fmt.Sprintf("    %q [label=\"%s\\n(%s)\", shape=ellipse, fillcolor=lightyellow];\n",
				name, name, typeName))
		}
	}
	if len(sorted) > 0 {
		output.WriteString("\n")
	}

	// Emit edges (outbound connections only, deduplicated)
	seen := map[string]bool{}
	var edges []string
	for _, r := range sorted {
		for _, conn := range r.Connections {
			if *conn.Direction != v20231001preview.DirectionOutbound {
				continue
			}
			targetName := resourceNameFromID(*conn.ID)
			edgeKey := *r.Name + "->" + targetName
			if seen[edgeKey] {
				continue
			}
			seen[edgeKey] = true
			edges = append(edges, fmt.Sprintf("    %q -> %q;\n",
				escapeDot(*r.Name), escapeDot(targetName)))
		}
	}
	sort.Strings(edges)
	for _, e := range edges {
		output.WriteString(e)
	}

	output.WriteString("}\n")
	return output.String()
}

// escapeDot escapes double-quote characters in a string for safe inclusion in DOT labels.
func escapeDot(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}

// resourceNameFromID extracts the resource name (last segment) from a Radius resource ID.
// Falls back to the raw ID string if parsing fails.
func resourceNameFromID(id string) string {
	parsed, err := resources.Parse(id)
	if err != nil {
		return id
	}
	name := parsed.Name()
	if name == "" {
		return id
	}
	return name
}
