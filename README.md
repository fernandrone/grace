# grace

[![Build Status](https://cloud.drone.io/api/badges/fernandrone/grace/status.svg)](https://cloud.drone.io/fernandrone/grace)
[![Go Report Card](https://goreportcard.com/badge/github.com/fernandrone/grace)](https://goreportcard.com/report/github.com/fernandrone/grace)

A command line tool that validates if containerized applications terminate gracefully.

## Supported Environments

> None! This is still in development. Come back later 🚧

_Work in Progress:_

* Docker
* Kubernetes

## Install

```console
go get github.com/fernandrone/grace/cmd/grace
```

## Usage

> This is a project in development. The instructions below are meant as documentation for the API design.

```console
$ ./hack/run-test-containers.sh
...
CONTAINER ID        IMAGE               COMMAND                   CREATED             STATUS                  PORTS               NAMES
bfda118d17f1        trapper:shell       "/bin/sh -c \"./trapp…"   1 second ago        Up Less than a second                       trapper-shell
3d873a7edb67        trapper:exec        "./trapper.sh"            1 second ago        Up Less than a second                       trapper-exec
$ grace trapper-exec trapper-shell
ID              IMAGE           COMMAND                         TERMINATION      EXIT CODE       DURATION
3d873a7edb67    trapper:exec    ./trapper.sh                    GracefulSuccess  0                 2s/10s
bfda118d17f1    trapper:shell   /bin/sh -c "./trapper.sh"       ForceKilled      137              10s/10s
```

To run from with a docker container, you will need to mount the host's Docker daemon socket:

```console:
$ ./hack/run-test-containers.sh
...
CONTAINER ID        IMAGE               COMMAND                   CREATED             STATUS                  PORTS               NAMES
bfda118d17f1        trapper:shell       "/bin/sh -c \"./trapp…"   1 second ago        Up Less than a second                       trapper-shell
3d873a7edb67        trapper:exec        "./trapper.sh"            1 second ago        Up Less than a second                       trapper-exec
$ docker build -t grace .
...
Step 11/11 : ENTRYPOINT ["/bin/grace"]
 ---> Running in 627b091ed3a4
Removing intermediate container 627b091ed3a4
 ---> d344d0820674
Successfully built d344d0820674
Successfully tagged grace:latest
$ docker run -v /var/run/docker.sock:/var/run/docker.sock -it grace:latest trapper-exec trapper-shell
ID              IMAGE           COMMAND                         TERMINATION      EXIT CODE       DURATION
3d873a7edb67    trapper:exec    ./trapper.sh                    GracefulSuccess  0                 2s/10s
bfda118d17f1    trapper:shell   /bin/sh -c "./trapper.sh"       ForceKilled      137              10s/10s
```

## Termination Values

| Value           | Description                                                                                                                                                                         |
| --------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GracefulSuccess | Ideally what you want to see everywhere. It means that the container terminated gracefully *and* the exit code was zero.                                                            |
| GracefulError   | This means that the container terminated gracefully but the exit code was not zero.                                                                                                 |
| ForceKilled     | The container did not terminate gracefully. Specifically, it failed to terminate within the allocated StopTimeout, triggering a SIGKILL by the container daemon.                    |
| OOMKilled       | The container did not terminate gracefully. During the shutdown it requested more memory than the limit allowed, triggering a SIGKILL by the container daemon.                      |
| Unhandled       | The container did not terminate gracefully. It terminated with status code 9 or 137 (which is reserved for SIGKILL) _but_ we did not detect neither an OOMKILL nor a timeout event. |
