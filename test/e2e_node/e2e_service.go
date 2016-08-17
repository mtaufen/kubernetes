/*
Copyright 2016 The Kubernetes Authors.

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

package e2e_node

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"

	"k8s.io/kubernetes/test/e2e/framework"
)

var serverStartTimeout = flag.Duration("server-start-timeout", time.Second*120, "Time to wait for each server to become healthy.")

type e2eService struct {
	killCmds []*killCmd
	rmDirs   []string

	context       *SharedContext
	etcdDataDir   string
	nodeName      string
	logFiles      map[string]logFileData
	cgroupsPerQOS bool
	evictionHard  string
}

type logFileData struct {
	files             []string
	journalctlCommand []string
}

const (
	// This is consistent with the level used in a cluster e2e test.
	LOG_VERBOSITY_LEVEL = "4"
	// Etcd binary is expected to either be available via PATH, or at this location.
	defaultEtcdPath = "/tmp/etcd"
)

func newE2eService(nodeName string, cgroupsPerQOS bool, evictionHard string, context *SharedContext) *e2eService {
	// Special log files that need to be collected for additional debugging.
	var logFiles = map[string]logFileData{
		"kern.log":       {[]string{"/var/log/kern.log"}, []string{"-k"}},
		"docker.log":     {[]string{"/var/log/docker.log", "/var/log/upstart/docker.log"}, []string{"-u", "docker"}},
		"cloud-init.log": {[]string{"/var/log/cloud-init.log"}, []string{"-u", "cloud*"}},
	}

	return &e2eService{
		context:       context,
		nodeName:      nodeName,
		logFiles:      logFiles,
		cgroupsPerQOS: cgroupsPerQOS,
		evictionHard:  evictionHard,
	}
}

func (es *e2eService) start() error {
	if _, err := getK8sBin("kubelet"); err != nil {
		return err
	}
	if _, err := getK8sBin("kube-apiserver"); err != nil {
		return err
	}

	cmd, err := es.startEtcd()
	if err != nil {
		return err
	}
	es.killCmds = append(es.killCmds, cmd)
	es.rmDirs = append(es.rmDirs, es.etcdDataDir)

	cmd, err = es.startApiServer()
	if err != nil {
		return err
	}
	es.killCmds = append(es.killCmds, cmd)

	cmd, err = es.startKubeletServer()
	if err != nil {
		return err
	}
	rcs, err := es.restartOnExit(cmd)
	if err != nil {
		return err
	}
	cmd.restartCmdChans = rcs
	es.killCmds = append(es.killCmds, cmd)
	es.rmDirs = append(es.rmDirs, es.context.PodConfigPath)

	return nil
}

// Get logs of interest either via journalctl or by creating sym links.
// Since we scp files from the remote directory, symlinks will be treated as normal files and file contents will be copied over.
func (es *e2eService) getLogFiles() {
	// Nothing to do if report dir is not specified.
	if framework.TestContext.ReportDir == "" {
		return
	}
	journaldFound := isJournaldAvailable()
	for targetFileName, logFileData := range es.logFiles {
		targetLink := path.Join(framework.TestContext.ReportDir, targetFileName)
		if journaldFound {
			// Skip log files that do not have an equivalent in journald based machines.
			if len(logFileData.journalctlCommand) == 0 {
				continue
			}
			out, err := exec.Command("sudo", append([]string{"journalctl"}, logFileData.journalctlCommand...)...).CombinedOutput()
			if err != nil {
				glog.Errorf("failed to get %q from journald: %v, %v", targetFileName, string(out), err)
			} else {
				if err = ioutil.WriteFile(targetLink, out, 0755); err != nil {
					glog.Errorf("failed to write logs to %q: %v", targetLink, err)
				}
			}
			continue
		}
		for _, file := range logFileData.files {
			if _, err := os.Stat(file); err != nil {
				// Expected file not found on this distro.
				continue
			}
			if err := copyLogFile(file, targetLink); err != nil {
				glog.Error(err)
			} else {
				break
			}
		}
	}
}

func copyLogFile(src, target string) error {
	// If not a journald based distro, then just symlink files.
	if out, err := exec.Command("sudo", "cp", src, target).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to copy %q to %q: %v, %v", src, target, out, err)
	}
	if out, err := exec.Command("sudo", "chmod", "a+r", target).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to make log file %q world readable: %v, %v", target, out, err)
	}
	return nil
}

func isJournaldAvailable() bool {
	_, err := exec.LookPath("journalctl")
	return err == nil
}

func (es *e2eService) stop() {
	for _, k := range es.killCmds {
		if err := k.Kill(); err != nil {
			glog.Errorf("Failed to stop %v: %v", k.name, err)
		}
	}
	for _, d := range es.rmDirs {
		err := os.RemoveAll(d)
		if err != nil {
			glog.Errorf("Failed to delete directory %s.\n%v", d, err)
		}
	}
}

func (es *e2eService) startEtcd() (*killCmd, error) {
	dataDir, err := ioutil.TempDir("", "node-e2e")
	if err != nil {
		return nil, err
	}
	es.etcdDataDir = dataDir
	var etcdPath string
	// CoreOS ships a binary named 'etcd' which is really old, so prefer 'etcd2' if it exists
	etcdPath, err = exec.LookPath("etcd2")
	if err != nil {
		etcdPath, err = exec.LookPath("etcd")
	}
	if err != nil {
		glog.Infof("etcd not found in PATH. Defaulting to %s...", defaultEtcdPath)
		_, err = os.Stat(defaultEtcdPath)
		if err != nil {
			return nil, fmt.Errorf("etcd binary not found")
		}
		etcdPath = defaultEtcdPath
	}
	cmd := exec.Command(etcdPath,
		"--listen-client-urls=http://0.0.0.0:2379,http://0.0.0.0:4001",
		"--advertise-client-urls=http://0.0.0.0:2379,http://0.0.0.0:4001")
	// Execute etcd in the data directory instead of using --data-dir because the flag sometimes requires additional
	// configuration (e.g. --name in version 0.4.9)
	cmd.Dir = es.etcdDataDir
	hcc := newHealthCheckCommand(
		"http://127.0.0.1:4001/v2/keys/", // Trailing slash is required,
		cmd,
		"etcd.log")
	return &killCmd{name: "etcd", cmd: cmd}, es.startServer(hcc)
}

func (es *e2eService) startApiServer() (*killCmd, error) {
	cmd := exec.Command("sudo", getApiServerBin(),
		"--etcd-servers", "http://127.0.0.1:4001",
		"--insecure-bind-address", "0.0.0.0",
		"--service-cluster-ip-range", "10.0.0.1/24",
		"--kubelet-port", "10250",
		"--allow-privileged", "true",
		"--v", LOG_VERBOSITY_LEVEL, "--logtostderr",
	)
	hcc := newHealthCheckCommand(
		"http://127.0.0.1:8080/healthz",
		cmd,
		"kube-apiserver.log")
	return &killCmd{name: "kube-apiserver", cmd: cmd}, es.startServer(hcc)
}

func (es *e2eService) startKubeletServer() (*killCmd, error) {
	dataDir, err := ioutil.TempDir("", "node-e2e-pod")
	if err != nil {
		return nil, err
	}
	es.context.PodConfigPath = dataDir
	var killOverride *exec.Cmd
	cmdArgs := []string{}
	if systemdRun, err := exec.LookPath("systemd-run"); err == nil {
		// On systemd services, detection of a service / unit works reliably while
		// detection of a process started from an ssh session does not work.
		// Since kubelet will typically be run as a service it also makes more
		// sense to test it that way
		unitName := fmt.Sprintf("kubelet-%d.service", rand.Int31())
		cmdArgs = append(cmdArgs, systemdRun, "--unit="+unitName, getKubeletServerBin())
		killOverride = exec.Command("sudo", "systemctl", "kill", unitName)
		es.logFiles["kubelet.log"] = logFileData{
			journalctlCommand: []string{"-u", unitName},
		}
	} else {
		cmdArgs = append(cmdArgs, getKubeletServerBin())
		cmdArgs = append(cmdArgs,
			"--runtime-cgroups=/docker-daemon",
			"--kubelet-cgroups=/kubelet",
			"--cgroup-root=/",
			"--system-cgroups=/system",
		)
	}
	cmdArgs = append(cmdArgs,
		"--api-servers", "http://127.0.0.1:8080",
		"--address", "0.0.0.0",
		"--port", "10250",
		"--hostname-override", es.nodeName, // Required because hostname is inconsistent across hosts
		"--volume-stats-agg-period", "10s", // Aggregate volumes frequently so tests don't need to wait as long
		"--allow-privileged", "true",
		"--serialize-image-pulls", "false",
		"--config", es.context.PodConfigPath,
		"--file-check-frequency", "10s", // Check file frequently so tests won't wait too long
		"--v", LOG_VERBOSITY_LEVEL, "--logtostderr",
		"--pod-cidr=10.180.0.0/24", // Assign a fixed CIDR to the node because there is no node controller.
		"--eviction-hard", es.evictionHard,
		"--eviction-pressure-transition-period", "30s",
	)
	if es.cgroupsPerQOS {
		cmdArgs = append(cmdArgs,
			"--cgroups-per-qos", "true",
		)
	}
	if !*disableKubenet {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cmdArgs = append(cmdArgs,
			"--network-plugin=kubenet",
			"--network-plugin-dir", filepath.Join(cwd, CNIDirectory, "bin")) // Enable kubenet
	}

	cmd := exec.Command("sudo", cmdArgs...)
	hcc := newHealthCheckCommand(
		"http://127.0.0.1:10255/healthz",
		cmd,
		"kubelet.log")
	return &killCmd{name: "kubelet", cmd: cmd, override: killOverride}, es.startServer(hcc)
}

func (es *e2eService) startServer(cmd *healthCheckCommand) error {
	cmdErrorChan := make(chan error)
	go func() {
		defer close(cmdErrorChan)

		// Create the output filename
		outPath := path.Join(framework.TestContext.ReportDir, cmd.outputFilename)
		outfile, err := os.Create(outPath)
		if err != nil {
			cmdErrorChan <- fmt.Errorf("Failed to create file %s for `%s` %v.", outPath, cmd, err)
			return
		}
		defer outfile.Close()
		defer outfile.Sync()

		// Set the command to write the output file
		cmd.Cmd.Stdout = outfile
		cmd.Cmd.Stderr = outfile

		// Death of this test process should kill the server as well.
		attrs := &syscall.SysProcAttr{}
		// Hack to set linux-only field without build tags.
		deathSigField := reflect.ValueOf(attrs).Elem().FieldByName("Pdeathsig")
		if deathSigField.IsValid() {
			deathSigField.Set(reflect.ValueOf(syscall.SIGTERM))
		} else {
			cmdErrorChan <- fmt.Errorf("Failed to set Pdeathsig field (non-linux build)")
			return
		}
		cmd.Cmd.SysProcAttr = attrs

		// Run the command
		err = cmd.Start()
		if err != nil {
			cmdErrorChan <- fmt.Errorf("%s Failed with error \"%v\".  Output written to: %s", cmd, err, outPath)
			return
		}
	}()

	endTime := time.Now().Add(*serverStartTimeout)
	for endTime.After(time.Now()) {
		select {
		// TODO(mtaufen): Now that we're using cmd.Start(), had to comment this out to prevent
		//                a read from a closed channel (cmdErrorChan).
		// case err := <-cmdErrorChan:
		// 	return err
		case <-time.After(time.Second):
			resp, err := http.Get(cmd.HealthCheckUrl)
			if err == nil && resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
	return fmt.Errorf("Timeout waiting for service %s", cmd)
}

func (es *e2eService) restartOnExit(k *killCmd) (*restartCmdChans, error) {
	q := make(chan int)   // Send to this channel to stop monitoring the KubeletServer.
	ack := make(chan int) // Recv from this channel to confirm that monitoring has stopped.
	rcs := &restartCmdChans{
		stopRestarting: q,
		ackStop:        ack,
	}

	if k.cmd == nil {
		return nil, fmt.Errorf("Cannot monitor a nil cmd")
	}
	if k.cmd.Process == nil {
		return nil, fmt.Errorf("Cannot monitor a nil Process")
	}
	go func() {
		for {
			// TODO(mtaufen): handle potential errors from Wait() and Run() more cleanly and completely
			// handle errors

			// Wait on Kubelet cmd here. If it exits, then we proceed.
			// TODO(mtaufen): Might be able to use cmd.Process.Wait() to get around the
			//                "wait already called" issue. Ultimately, cmd.Process.Wait()
			//                relies on the wait4 syscall, and I haven't looked deep enough
			//                to figure out if you can only do one of those on a given process.
			err := k.cmd.Wait()
			if err != nil {
				// TODO(mtaufen): I'm getting a ton of "could not wait on process". Why? A TON means it
				//                is either because the restart isn't working or isn't being reached.
				//                It's being reached but it's not working. The last thing in the Kubelet log
				//                is that the Kubelet saw a new configuration on the API server.
				//                The test somehow gets another poll in to the Kubelet... even though no new logs.
				glog.Errorf("Could not wait on process: %#v, Error: %v", k.cmd.Process, err)
			}
			time.Sleep(1 * time.Second) // TODO(mtaufen): Wait 10 seconds before restarting the Kubelet

			select {
			case <-q:
				ack <- 0
				return
			default:
				// We only get here if the Kubelet died, restart it.
				// Need to create a new Cmd object to call Start again,
				// because this Cmd thinks it was already started.

				// TODO(mtaufen): And we probably want to add the healthz check to this as well

				// TODO(mtaufen): Find a better way to copy this?
				// Only need to copy the following to restart Kubelet, will need
				// to investigate other functions (e.g. startEtcd) to see if additional
				// things need to be copied:
				// Path - set by exec.Command
				// Args - set by exec.Command
				// Stderr - set by startServer
				// Stdout - set by startServer
				// SysProcAttr - set by startServer
				// TODO(mtaufen): Might need to copy more things e.g. Dir, if we want this to work
				//                for more than just Kubelet

				glog.Infof("Kubelet exited with state: %s, restarting cmd: %+v.", k.cmd.ProcessState.String(), k.cmd)

				k.cmd = &exec.Cmd{
					Path:        k.cmd.Path,
					Args:        k.cmd.Args,
					Env:         k.cmd.Env,
					Stdout:      k.cmd.Stdout,
					Stderr:      k.cmd.Stderr,
					SysProcAttr: k.cmd.SysProcAttr, // TODO(mtaufen): Maybe try &syscall.SysProcAttr{}?
				}

				// TODO(mtaufen): It's exiting poorly because we're pushing a bad config in the test.
				// I think. We're pulling down a configuration with legit values, but they are falling
				// to defaults somewhere before we get it into an actual go struct.

				// Start the command
				err := k.cmd.Start() // TODO(mtaufen): What should I do when this fails? Keep trying to restart? For now a failed restart is fatal.
				if err != nil {
					glog.Fatalf("Could not restart Cmd: %#v, Error: %v", k.cmd, err)
				}

			}
		}
	}()
	return rcs, nil
}

// killCmd is a struct to kill a given cmd. The cmd member specifies a command
// to find the pid of and attempt to kill.
// If the override field is set, that will be used instead to kill the command.
// name is only used for logging
type killCmd struct {
	name            string
	cmd             *exec.Cmd
	override        *exec.Cmd
	restartCmdChans *restartCmdChans
}

type restartCmdChans struct {
	stopRestarting chan int
	ackStop        chan int
}

func (k *killCmd) Kill() error {
	name := k.name
	cmd := k.cmd

	if k.override != nil {
		return k.override.Run()
	}

	if cmd == nil {
		return fmt.Errorf("Could not kill %s because both `override` and `cmd` are nil", name)
	}

	if cmd.Process == nil {
		glog.V(2).Infof("%s not running", name)
		return nil
	}
	pid := cmd.Process.Pid
	if pid <= 1 {
		return fmt.Errorf("invalid PID %d for %s", pid, name)
	}

	// Stop any restart loops prior to trying to shut down the process.
	if k.restartCmdChans != nil {
		k.restartCmdChans.stopRestarting <- 0
		<-k.restartCmdChans.ackStop // TODO(mtaufen): Need to wait on the ack after trying to kill the
	}

	// Attempt to shut down the process in a friendly manner before forcing it.
	waitChan := make(chan error)
	go func() {
		_, err := cmd.Process.Wait()
		waitChan <- err
		close(waitChan)
	}()

	const timeout = 10 * time.Second
	for _, signal := range []string{"-TERM", "-KILL"} {
		glog.V(2).Infof("Killing process %d (%s) with %s", pid, name, signal)
		cmd := exec.Command("sudo", "kill", signal, strconv.Itoa(pid))

		// Run the 'kill' command in a separate process group so sudo doesn't ignore it
		attrs := &syscall.SysProcAttr{}
		// Hack to set unix-only field without build tags.
		setpgidField := reflect.ValueOf(attrs).Elem().FieldByName("Setpgid")
		if setpgidField.IsValid() {
			setpgidField.Set(reflect.ValueOf(true))
		} else {
			return fmt.Errorf("Failed to set Setpgid field (non-unix build)")
		}
		cmd.SysProcAttr = attrs

		_, err := cmd.Output()
		if err != nil {
			glog.Errorf("Error signaling process %d (%s) with %s: %v", pid, name, signal, err)
			continue
		}

		select {
		case err := <-waitChan:
			if err != nil {
				return fmt.Errorf("error stopping %s: %v", name, err)
			}
			// Success!
			return nil
		case <-time.After(timeout):
			// Continue.
		}
	}

	return fmt.Errorf("unable to stop %s", name)
}

type healthCheckCommand struct {
	*exec.Cmd
	HealthCheckUrl string
	outputFilename string
}

func newHealthCheckCommand(healthCheckUrl string, cmd *exec.Cmd, filename string) *healthCheckCommand {
	return &healthCheckCommand{
		HealthCheckUrl: healthCheckUrl,
		Cmd:            cmd,
		outputFilename: filename,
	}
}

func (hcc *healthCheckCommand) String() string {
	return fmt.Sprintf("`%s` health-check: %s", strings.Join(append([]string{hcc.Path}, hcc.Args[1:]...), " "), hcc.HealthCheckUrl)
}
