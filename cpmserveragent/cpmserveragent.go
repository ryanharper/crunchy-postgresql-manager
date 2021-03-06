/*
 Copyright 2015 Crunchy Data Solutions, Inc.
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

package cpmserveragent

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/crunchydata/crunchy-postgresql-manager/logit"
	"github.com/crunchydata/crunchy-postgresql-manager/util"
	dockerapi "github.com/fsouza/go-dockerclient"
	"net/rpc"
	"os/exec"
	"strings"
)

type InspectOutput struct {
	IPAddress    string
	RunningState string
}

type Args struct {
	A, B, C, D, E, CPU, MEM, CommandPath string
}
type DockerRunArgs struct {
	CPU           string
	MEM           string
	ClusterID     string
	ServerID      string
	Image         string
	IPAddress     string
	Standalone    string
	PGDataPath    string
	ContainerName string
	ContainerType string
	CommandOutput string
	CommandPath   string
	EnvVars       map[string]string
}

type Command struct {
	Output string
}

type InspectCommandOutput struct {
	IPAddress    string
	RunningState string
}

/*
 args.A contains the name of what we are going to execute
 such as 'iostat.sh' or 'df.sh'
*/
func (t *Command) Get(args *Args, reply *Command) error {

	logit.Info.Println("on server, Command Get called A=" + args.A + " B=" + args.B)
	if args.A == "" {
		logit.Error.Println("A was nil")
		return errors.New("Arg A was nil")
	}
	if args.B == "" {
		logit.Info.Println("B was nil")
	} else {
		logit.Info.Println("B was " + args.B)
	}

	var cmd *exec.Cmd

	if args.B == "" {
		cmd = exec.Command(args.A)
	} else {
		cmd = exec.Command(args.A, args.B)
	}
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logit.Error.Println(err.Error())
		errorString := fmt.Sprintf("%s\n%s\n%s\n", err.Error(), out.String(), stderr.String())
		return errors.New(errorString)
	}
	logit.Info.Println("command output was " + out.String())
	reply.Output = out.String()

	return nil
}

//
// DockerInspectCommand is to be run on the server that is running
// docker, it will connnect to docker via the unix domain socket
//
func (t *Command) DockerInspectCommand(args *Args, reply *Command) error {

	logit.Info.Println("DockerInspectCommand called A=" + args.A)
	if args.A == "" {
		logit.Error.Println("A was nil")
		return errors.New("Arg A was nil")
	}

	var cmd *exec.Cmd

	cmd = exec.Command("docker",
		"inspect",
		"--format",
		"{{ .NetworkSettings.IPAddress }}",
		args.A)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logit.Error.Println(err.Error())
		errorString := fmt.Sprintf("%s\n%s\n%s\n", err.Error(), out.String(), stderr.String())
		return errors.New(errorString)
	}

	ipaddr := strings.Trim(out.String(), "\n")
	logit.Info.Println("docker inspect command output was " + ipaddr)
	reply.Output = ipaddr

	return nil
}

//
// DockerRemoveCommand is to be run on the server that is running
// docker, it will connnect to docker via the unix domain socket
// and remove an existing container
//
func (t *Command) DockerRemoveCommand(args *Args, reply *Command) error {

	logit.Info.Println("DockerRemoveCommand called A=" + args.A)
	if args.A == "" {
		logit.Error.Println("A was nil")
		return errors.New("Arg A was nil")
	}

	var containerName = args.A

	//if a container exists with that name, then we need
	//to stop it first and then remove it
	//inspect
	//stop
	//remove
	docker, err := dockerapi.NewClient("unix://var/run/docker.sock")
	if err != nil {
		logit.Error.Println("can't get connection to docker socket")
		return err
	}

	//if we can't inspect the container, then we give up
	//on trying to remove it, this is ok to pass since
	//a user can remove the container manually
	container, err3 := docker.InspectContainer(containerName)
	if err3 != nil {
		logit.Error.Println("can't inspect container " + containerName)
		logit.Error.Println("inspect container error was " + err3.Error())
		return nil
	}

	if container != nil {
		logit.Info.Println("container found during inspect")
		err3 = docker.StopContainer(containerName, 10)
		if err3 != nil {
			logit.Info.Println("can't stop container " + containerName)
		}
		logit.Info.Println("container stopped ")
		opts := dockerapi.RemoveContainerOptions{ID: containerName}
		err := docker.RemoveContainer(opts)
		if err != nil {
			logit.Info.Println("can't remove container " + containerName)
		}
		logit.Info.Println("container removed ")
	}

	reply.Output = "success"

	return nil
}

//
// DockerStartCommand is to be run on the server that is running
// docker, it will connnect to docker via the unix domain socket
// and start an existing container
//
func (t *Command) DockerStartCommand(args *Args, reply *Command) error {

	logit.Info.Println("DockerStartCommand called A=" + args.A)
	if args.A == "" {
		logit.Error.Println("A was nil")
		return errors.New("Arg A was nil")
	}

	var containerName = args.A

	//if a container exists with that name, then we need
	//to stop it first and then remove it
	//inspect
	//start
	docker, err := dockerapi.NewClient("unix://var/run/docker.sock")
	if err != nil {
		logit.Error.Println("can't get connection to docker socket")
		return err
	}

	//if we can't inspect the container, then we give up
	//on trying to start it
	container, err3 := docker.InspectContainer(containerName)
	if err3 != nil {
		logit.Error.Println("can't inspect container " + containerName)
		logit.Error.Println("inspect container error was " + err3.Error())
		return errors.New("container " + containerName + " not found")
	}

	if container != nil {
		logit.Info.Println("container found during inspect")

		if container.State.Running {
			logit.Info.Println("container " + containerName + " was already running, no need to start it")
			return nil
		}

		err3 = docker.StartContainer(containerName, nil)
		if err3 != nil {
			logit.Error.Println("can't start container " + containerName)
			return errors.New("can not start container " + containerName)
		}
		logit.Info.Println("container started ")
	}

	reply.Output = "success"

	return nil
}

//
// DockerStopCommand is to be run on the server that is running
// docker, it will connnect to docker via the unix domain socket
// and stop an existing container in a running state
//
func (t *Command) DockerStopCommand(args *Args, reply *Command) error {

	logit.Info.Println("DockerStopCommand called A=" + args.A)
	if args.A == "" {
		logit.Error.Println("A was nil")
		return errors.New("Arg A was nil")
	}

	var containerName = args.A

	docker, err := dockerapi.NewClient("unix://var/run/docker.sock")
	if err != nil {
		logit.Error.Println("can't get connection to docker socket")
		return err
	}

	//if we can't inspect the container, then we give up
	//on trying to stop it
	container, err3 := docker.InspectContainer(containerName)
	if err3 != nil {
		logit.Info.Println("during stop, can't inspect container " + containerName)
		return nil
	}

	if container != nil {
		logit.Info.Println("container found during inspect")
		var timeout uint
		timeout = 10
		err3 = docker.StopContainer(containerName, timeout)
		if err3 != nil {
			logit.Info.Println("can't stop container " + containerName)
		}
		logit.Info.Println("container " + containerName + " stopped ")
	}

	reply.Output = "success"

	return nil
}

//
// DockerInspectFullCommand is to be run on the server that is running
// docker, it will connnect to docker via the unix domain socket
//
func (t *Command) DockerInspect2Command(args *Args, reply *InspectCommandOutput) error {

	logit.Info.Println("DockerInspect2Command called containerName=" + args.A)
	if args.A == "" {
		logit.Error.Println("containerName was nil")
		return errors.New("containerName was nil")
	}

	var containerName = args.A

	docker, err := dockerapi.NewClient("unix://var/run/docker.sock")
	if err != nil {
		logit.Error.Println("can't get connection to docker socket")
		return err
	}

	//if we can't inspect the container, then we give up
	//on trying to stop it
	reply.RunningState = "down"
	reply.IPAddress = ""

	container, err3 := docker.InspectContainer(containerName)
	if err3 != nil {
		logit.Info.Println("can't inspect container " + containerName)
		return err3
	}

	if container != nil {
		logit.Info.Println("container found during inspect")
		if container.State.Running {
			reply.RunningState = "up"
			logit.Info.Println("container status is up")
			logit.Info.Println("container ipaddress is " + container.NetworkSettings.IPAddress)
			reply.IPAddress = container.NetworkSettings.IPAddress
		} else {
			reply.RunningState = "down"
			logit.Info.Println("container status is down")
		}
	}

	return nil
}

func (t *Command) DockerRun(args *DockerRunArgs, reply *Command) error {

	logit.Info.Println("on server, Command DockerRun called CommandPath=" + args.CommandPath + " PGDataPath=" + args.PGDataPath + " ContainerName=" + args.ContainerName + " Image=" + args.Image + " cpm=" + args.CPU + " mem=" + args.MEM)

	var cmd *exec.Cmd

	var allEnvVars = ""
	if args.EnvVars != nil {
		for k, v := range args.EnvVars {
			allEnvVars = allEnvVars + " -e " + k + "=" + v
		}
	}
	logit.Info.Println("env vars " + allEnvVars)

	cmd = exec.Command(args.CommandPath, args.PGDataPath, args.ContainerName, args.Image, args.CPU, args.MEM, allEnvVars)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logit.Error.Println(err.Error())
		errorString := fmt.Sprintf("%s\n%s\n%s\n", err.Error(), out.String(), stderr.String())
		return errors.New(errorString)
	}
	logit.Info.Println("command output was " + out.String())
	reply.Output = out.String()

	return nil
}

//used by backup to provision backup directories
//used by adminapi clustermgmt to delete a disk volume
//used by adminapi nodemgmt to delete a disk volume
//used by adminapi provision to provision a disk volume
//used by adminapi server to get server metrics
//used by monserver to collect cpu and mem stats
func AgentCommand(arga string, argb string, ipaddress string) (string, error) {
	client, err := rpc.DialHTTP("tcp", ipaddress+":13000")
	if err != nil {
		logit.Error.Println("AgentCommand: dialing:" + err.Error())
	}
	if client == nil {
		logit.Error.Println("AgentCommand: dialing: client was nil")
		return "", errors.New("client was nil")
	}

	var command Command

	args := &Args{}
	args.A = util.GetBase() + "/bin/" + arga
	args.B = argb
	err = client.Call("Command.Get", args, &command)
	if err != nil {
		logit.Error.Println("AgentCommand:" + arga + " Command Get error:" + err.Error())
		return "", err
	}
	return command.Output, nil
}

//used by cpm-backup to create backup jobs
//used by adminapi provision to provision container
func AgentDockerRun(args DockerRunArgs, ipaddress string) (string, error) {
	logit.Info.Println("server.go:AgentDockerRun:cpu=" + args.CPU + " mem=" + args.MEM)
	if args.EnvVars != nil {
		for k, v := range args.EnvVars {
			logit.Info.Println("server.go: env var " + k + " " + v)
		}
	}
	client, err := rpc.DialHTTP("tcp", ipaddress+":13000")
	if err != nil {
		logit.Error.Println("AgentDockerRun dialing:" + err.Error())
		return "", err
	}
	if client == nil {
		logit.Error.Println("AgentDockerRun dialing: client was nil")
		return "", errors.New("client was nil")
	}

	var command Command

	err = client.Call("Command.DockerRun", args, &command)
	if err != nil {
		logit.Error.Println("DockerRun error:" + err.Error())
		return "", err
	}
	return command.Output, nil
}

func DockerInspectCommand(arga string, ipaddress string) (string, error) {
	client, err := rpc.DialHTTP("tcp", ipaddress+":13000")
	if err != nil {
		logit.Error.Println("DockerInspectCommand: dialing:" + err.Error())
		return "", err
	}
	if client == nil {
		logit.Error.Println("DockerInspectCommand: dialing: client was nil")
		return "", errors.New("client was nil")
	}

	var command Command

	args := &Args{}
	args.A = arga
	err = client.Call("Command.DockerInspectCommand", args, &command)
	if err != nil {
		logit.Error.Println("DockerInspectCommand:" + arga + " Command DockerInspectCommand error:" + err.Error())
		return "", err
	}
	return command.Output, nil
}

//used by adminap nodemgmt
func DockerInspect2Command(arga string, ipaddress string) (InspectOutput, error) {
	var command InspectCommandOutput
	var output InspectOutput

	client, err := rpc.DialHTTP("tcp", ipaddress+":13000")
	if err != nil {
		logit.Error.Println("DockerInspect2Command: dialing:" + err.Error())
		return output, err
	}
	if client == nil {
		logit.Error.Println("DockerInspect2Command dialing: client was nil")
		return output, errors.New("client was nil here")
	}

	args := &Args{}
	args.A = arga
	err = client.Call("Command.DockerInspect2Command", args, &command)
	if err != nil {
		logit.Error.Println("DockerInspect2Command:" + arga + " Command DockerInspect2Command error:" + err.Error())
		return output, err
	}
	output.IPAddress = command.IPAddress
	output.RunningState = command.RunningState

	return output, nil
}

//used by adminapi clustermgmt to remove a container
//used by adminapi nodemtm to remove a container
//used by adminapi provision to remove a container
func DockerRemoveContainer(arga string, ipaddress string) (string, error) {
	client, err := rpc.DialHTTP("tcp", ipaddress+":13000")
	if err != nil {
		logit.Error.Println("DockerRemoveContainer: dialing:" + err.Error())
		return "", err
	}
	if client == nil {
		logit.Error.Println("DockerRemoveContainer: dialing:" + err.Error())
		return "", errors.New("client was nil here2")
	}

	var command Command

	args := &Args{}
	args.A = arga
	err = client.Call("Command.DockerRemoveCommand", args, &command)
	if err != nil {
		logit.Error.Println("DockerRemoveCommand arga error:" + err.Error())
		return "", err
	}
	return command.Output, nil
}

//used by adminapi nodemgmt
func DockerStartContainer(containerName string, ipaddress string) (string, error) {
	client, err := rpc.DialHTTP("tcp", ipaddress+":13000")
	if err != nil {
		logit.Error.Println("DockerStartContainer dialing:" + err.Error())
		return "", err
	}
	if client == nil {
		logit.Error.Println("DockerStartContainer dialing:" + err.Error())
		return "", errors.New("client was nil 3")
	}

	var command Command

	args := &Args{}
	args.A = containerName
	err = client.Call("Command.DockerStartCommand", args, &command)
	if err != nil {
		logit.Error.Println("DockerStartContainer containerName error:" + err.Error())
		return "", err
	}
	return command.Output, nil
}

//used by adminapi nodemgmt
func DockerStopContainer(containerName string, ipaddress string) (string, error) {

	client, err := rpc.DialHTTP("tcp", ipaddress+":13000")
	if err != nil {
		logit.Error.Println("DockerStopContainer dialing:" + err.Error())
		return "", err
	}
	if client == nil {
		logit.Error.Println("DockerStopContainer dialing:" + err.Error())
		return "", errors.New("client was nil here 4")
	}

	var command Command

	args := &Args{}
	args.A = containerName
	err = client.Call("Command.DockerStopCommand", args, &command)
	if err != nil {
		logit.Info.Println("DockerStopContainer containerName error:" + err.Error())
		return "", err
	}
	return command.Output, nil
}
