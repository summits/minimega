// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"fmt"
	"minicli"
	log "minilog"
	"os"
)

type dotVM struct {
	Vlans []string
	State string
	Text  string
}

var stateToColor = map[VMState]string{
	VM_BUILDING: "yellow",
	VM_RUNNING:  "green",
	VM_PAUSED:   "yellow",
	VM_QUIT:     "blue",
	VM_ERROR:    "red",
}

var dotCLIHandlers = []minicli.Handler{
	{ // viz
		HelpShort: "visualize the current experiment as a graph",
		HelpLong: `
Output the current experiment topology as a graphviz readable 'dot' file.`,
		Patterns: []string{
			"viz <filename>",
		},
		Call: wrapSimpleCLI(cliDot),
	},
}

func init() {
	registerHandlers("dot", dotCLIHandlers)
}

// dot returns a graphviz 'dotfile' string representing the experiment topology
// from the perspective of this node.
func cliDot(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	// Create the file before running any commands incase there is an error
	fout, err := os.Create(c.StringArgs["filename"])
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	defer fout.Close()

	writer := bufio.NewWriter(fout)
	fmt.Fprintln(writer, "graph minimega {")
	fmt.Fprintln(writer, `size="8,11";`)
	fmt.Fprintln(writer, "overlap=false;")
	//fmt.Fprintf(fout, "Legend [shape=box, shape=plaintext, label=\"total=%d\"];\n", len(n.effectiveNetwork))

	vlans := make(map[int]bool)

	info, _ := globalVmInfo(nil, nil)
	for host, vms := range info {
		for _, vm := range vms {
			info, err := vm.Info([]string{"ip", "ip6"})
			if err != nil || len(info) != 2 {
				// Should never happen
				log.Error("bad VM info: %v -- %v", host, vm.Name)
				continue
			}

			text := fmt.Sprintf(`"%v:%v:%v:%v:%v"`, host, vm.Name, vm.ID, info[0], info[1])
			color := stateToColor[vm.State()]

			fmt.Fprintf(writer, "%v [style=filled, color=%v];\n", text, color)

			for _, net := range vm.Config().Networks {
				fmt.Fprintf(writer, "%v -- %v\n", text, net.VLAN)
				vlans[net.VLAN] = true
			}
		}
	}

	for vlan := range vlans {
		fmt.Fprintf(writer, "%v;\n", vlan)
	}

	fmt.Fprint(writer, "}")
	if err = writer.Flush(); err != nil {
		resp.Error = err.Error()
	}

	return resp
}
