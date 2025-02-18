// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"math/rand"
	log "minilog"
	"os"
	"path/filepath"
	"ranges"
	"strconv"
	"sync"
	"time"
)

// total list of vms running on this host
type VMs map[int]VM

// apply applies the provided function to the vm in VMs whose name or ID
// matches the provided vm parameter.
func (vms VMs) apply(idOrName string, fn func(VM) error) error {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return vmNotFound(idOrName)
	}
	return fn(vm)
}

func saveConfig(ns string, fns map[string]VMConfigFns, configs interface{}) []string {
	var cmds = []string{}

	for k, fns := range fns {
		if fns.PrintCLI != nil {
			if v := fns.PrintCLI(configs); len(v) > 0 {
				cmds = append(cmds, v)
			}
		} else if v := fns.Print(configs); len(v) > 0 {
			cmds = append(cmds, fmt.Sprintf("vm %s config %s %s", ns, k, v))
		}
	}

	return cmds
}

func (vms VMs) save(file *os.File, args []string) error {
	var allVms bool
	for _, vm := range args {
		if vm == Wildcard {
			allVms = true
			break
		}
	}

	if allVms && len(args) != 1 {
		log.Debug("ignoring vm names, wildcard is present")
	}

	var toSave []string
	if allVms {
		for k, _ := range vms {
			toSave = append(toSave, fmt.Sprintf("%v", k))
		}
	} else {
		toSave = args
	}

	for _, vmStr := range toSave { // iterate over the vm id's specified
		vm := vms.findVm(vmStr)
		if vm == nil {
			return fmt.Errorf("vm %v not found", vm)
		}

		// build up the command list to re-launch this vm, first clear all
		// previous configuration.
		cmds := []string{"clear vm config"}

		cmds = append(cmds, saveConfig("", baseConfigFns, vm.Config())...)

		switch vm := vm.(type) {
		case *vmKVM:
			cmds = append(cmds, "vm config kvm true")
			cmds = append(cmds, saveConfig("kvm", kvmConfigFns, &vm.KVMConfig)...)
		default:
		}

		if vm.Name() != "" {
			cmds = append(cmds, "vm launch "+vm.Name())
		} else {
			cmds = append(cmds, "vm launch 1")
		}

		// and a blank line
		cmds = append(cmds, "")

		// write commands to file
		for _, cmd := range cmds {
			_, err := file.WriteString(cmd + "\n")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (vms VMs) qmp(idOrName, qmp string) (string, error) {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return "", vmNotFound(idOrName)
	}

	if vm, ok := vm.(*vmKVM); ok {
		return vm.QMPRaw(qmp)
	} else {
		// TODO
		return "", fmt.Errorf("`%s` is not a kvm vm -- command unsupported", vm.Name())
	}
}

func (vms VMs) screenshot(idOrName, path string, max int) error {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return vmNotFound(idOrName)
	}
	kvm, ok := vm.(*vmKVM)
	if !ok {
		return fmt.Errorf("`%s` is not a kvm vm -- command unsupported", vm.Name())
	}

	suffix := rand.New(rand.NewSource(time.Now().UnixNano())).Int31()
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("minimega_screenshot_%v", suffix))

	err := kvm.q.Screendump(tmp)
	if err != nil {
		return err
	}

	err = ppmToPng(tmp, path, max)
	if err != nil {
		return err
	}

	err = os.Remove(tmp)
	if err != nil {
		return err
	}

	return nil
}

func (vms VMs) migrate(idOrName, filename string) error {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return vmNotFound(idOrName)
	}
	kvm, ok := vm.(*vmKVM)
	if !ok {
		return fmt.Errorf("`%s` is not a kvm vm -- command unsupported", vm.Name())
	}

	return kvm.Migrate(filename)
}

func (vms VMs) findVm(idOrName string) VM {
	id, err := strconv.Atoi(idOrName)
	if err != nil {
		// Search for VM by name
		for _, v := range vms {
			if v.Name() == idOrName {
				return v
			}
		}
	}

	return vms[id]
}

// launch one or more vms. this will copy the info struct, one per vm
// and launch each one in a goroutine. it will not return until all
// vms have reported that they've launched.
func (vms VMs) launch(name string, vmType VMType, ack chan int) error {
	// Make sure that there isn't another VM with the same name
	if name != "" {
		for _, vm := range vms {
			if vm.Name() == name {
				return fmt.Errorf("vm launch duplicate VM name: %s", name)
			}
		}
	}

	var vm VM
	switch vmType {
	case KVM:
		vm = NewKVM()
	default:
		// TODO
	}

	return vm.Launch(name, ack)
}

func (vms VMs) start(target string) []error {
	return expandVmTargets(target, true, func(vm VM, wild bool) (bool, error) {
		if wild && vm.State()&(VM_PAUSED|VM_BUILDING) != 0 {
			// If wild, we only start VMs in the building or running state
			return true, vm.Start()
		} else if !wild && vm.State()&VM_RUNNING == 0 {
			// If not wild, start VMs that aren't already running
			return true, vm.Start()
		}

		return false, nil
	})
}

func (vms VMs) stop(target string) []error {
	return expandVmTargets(target, true, func(vm VM, _ bool) (bool, error) {
		if vm.State()&VM_RUNNING != 0 {
			return true, vm.Stop()
		}

		return false, nil
	})
}

func (vms VMs) kill(target string) []error {
	killedVms := map[int]bool{}

	errs := expandVmTargets(target, false, func(vm VM, _ bool) (bool, error) {
		if vm.State()&(VM_QUIT|VM_ERROR) != 0 {
			return false, nil
		}

		vm.Kill()
		killedVms[vm.ID()] = true
		return true, nil
	})

outer:
	for len(killedVms) > 0 {
		select {
		case id := <-killAck:
			log.Info("VM %v killed", id)
			delete(killedVms, id)
		case <-time.After(COMMAND_TIMEOUT * time.Second):
			log.Error("vm kill timeout")
			break outer
		}
	}

	for id := range killedVms {
		log.Info("VM %d failed to acknowledge kill", id)
	}

	return errs
}

func (vms VMs) flush() {
	for i, vm := range vms {
		if vm.State()&(VM_QUIT|VM_ERROR) != 0 {
			log.Infoln("deleting VM: ", i)
			delete(vms, i)
		}
	}
}

func (vms VMs) info(vmType string) ([]string, [][]string, error) {
	table := make([][]string, 0, len(vms))

	masks := vmMasks
	if vmType == "kvm" {
		masks = kvmMasks
	}

	for _, vm := range vms {
		var row []string
		var err error

		// All VMs
		if vmType == "" {
			row, err = vm.Info(masks)
		} else if vm, ok := vm.(*vmKVM); ok && vmType == "kvm" {
			row, err = vm.Info(masks)
		}

		if err != nil {
			continue
		}

		table = append(table, row)
	}

	return masks, table, nil
}

// cleanDirs removes all isntance directories in the minimega base directory
func (vms VMs) cleanDirs() {
	log.Debugln("cleanDirs")
	for _, vm := range vms {
		if vm, ok := vm.(*vmKVM); ok {
			log.Debug("cleaning instance path: %v", vm.instancePath)
			err := os.RemoveAll(vm.instancePath)
			if err != nil {
				log.Error("clearDirs: %v", err)
			}
		} else {
			// TODO
		}
	}
}

// expandVmTargets is the fan out/in method to apply a function to a set of VMs
// specified by target. Specifically, it:
//
// 	1. Expands target to a list of VM names and IDs (or wild)
// 	2. Invokes fn on all the matching VMs
// 	3. Collects all the errors from the invoked fns
// 	4. Records in the log a list of VMs that were not found
//
// The fn that is passed in takes two arguments: the VM struct and a boolean
// specifying whether the invocation was wild or not. The fn returns a boolean
// that indicates whether the target was applicable (e.g. calling start on an
// already running VM would not be applicable) and an error.
//
// The concurrent boolean controls whether fn is run concurrently on multiple
// VMs or not. If the fns alter state they can set this flag to false rather
// than dealing with locking.
func expandVmTargets(target string, concurrent bool, fn func(VM, bool) (bool, error)) []error {
	names := map[string]bool{} // Names of VMs for which to apply fn
	ids := map[int]bool{}      // IDs of VMs for which to apply fn

	vals, err := ranges.SplitList(target)
	if err != nil {
		return []error{err}
	}
	for _, v := range vals {
		id, err := strconv.Atoi(v)
		if err == nil {
			ids[id] = true
		} else {
			names[v] = true
		}
	}
	wild := hasWildcard(names)
	delete(names, Wildcard)

	// wg determine when it's okay to close errChan
	var wg sync.WaitGroup
	errChan := make(chan error)

	// lock prevents concurrent writes to results
	var lock sync.Mutex
	results := map[string]bool{}

	// Wrap function with magic
	magicFn := func(vm VM) {
		defer wg.Done()
		ok, err := fn(vm, wild)
		if err != nil {
			errChan <- err
		}

		lock.Lock()
		defer lock.Unlock()
		results[vm.Name()] = ok
	}

	for _, vm := range vms {
		if wild || names[vm.Name()] || ids[vm.ID()] {
			delete(names, vm.Name())
			delete(ids, vm.ID())
			wg.Add(1)

			// Use concurrency only if requested
			if concurrent {
				go magicFn(vm)
			} else {
				magicFn(vm)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	var errs []error

	for err := range errChan {
		errs = append(errs, err)
	}

	// Special cases: specified one VM and
	//   1. it wasn't found
	//   2. it wasn't a valid target (e.g. start already running VM)
	if len(vals) == 1 && !wild {
		if (len(names) + len(ids)) == 1 {
			errs = append(errs, fmt.Errorf("VM not found: %v", vals[0]))
		} else if !results[vals[0]] {
			errs = append(errs, fmt.Errorf("VM state error: %v", vals[0]))
		}
	}

	// Log the names/ids of the vms that weren't found
	if (len(names) + len(ids)) > 0 {
		vals := []string{}
		for v := range names {
			vals = append(vals, v)
		}
		for v := range ids {
			vals = append(vals, strconv.Itoa(v))
		}
		log.Info("VMs not found: %v", vals)
	}

	return errs
}
