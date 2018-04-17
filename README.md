![rxtx data transmission](mast-logo.jpg)
[![irsync Release](https://img.shields.io/github/release/cjimti/rtbeat.svg)](https://github.com/cjimti/rtbeat/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/cjimti/rtbeat)](https://goreportcard.com/report/github.com/cjimti/rtbeat)


[![Docker Container Image Size](https://shields.beevelop.com/docker/image/image-size/cjimti/rtbeat/latest.svg)](https://hub.docker.com/r/cjimti/irsync/)
[![Docker Container Layers](https://shields.beevelop.com/docker/image/layers/cjimti/rtbeat/latest.svg)](https://hub.docker.com/r/cjimti/rtbeat/)
[![Docker Container Pulls](https://img.shields.io/docker/pulls/cjimti/rtbeat.svg)](https://hub.docker.com/r/cjimti/rtbeat/)

# Rtbeat

[Rtbeat](https://github.com/cjimti/rtbeat) processes HTTP POST data from [rxtx](https://github.com/cjimti/rxtx) and publishes events into [elasticsearch], [logstash], [kafka], [redis] or directly to log files.

# Rtbeat Development

### Requirements

* [Golang](https://golang.org/dl/) 1.7 or greater.

### Clone

To clone Rtbeat from the git repository, run the following commands:

```
mkdir -p ${GOPATH}/src/github.com/cjimti/rtbeat
git clone https://github.com/cjimti/rtbeat ${GOPATH}/src/github.com/cjimti/rtbeat
```

For further development, check out the [beat developer guide](https://www.elastic.co/guide/en/beats/libbeat/current/new-beat.html).

### Build

To build the binary for Rtbeat run the command below. This will generate a binary
in the same directory with the name rtbeat.

```
make
```

### Run

To run Rtbeat with debugging output enabled, run:

```
./rtbeat -c rtbeat.yml -e -d "*"
```

### Test

To test Rtbeat, run the following command:

```
make testsuite
```

alternatively:
```
make unit-tests
make system-tests
make integration-tests
make coverage-report
```

The test coverage is reported in the folder `./build/coverage/`

### Update

Each beat has a template for the mapping in elasticsearch and a documentation for the fields
which is automatically generated based on `fields.yml` by running the following command.

```
make update
```

### Cleanup

To clean up the build directory and generated artifacts, run:

```
make clean
```

## Packaging

The beat frameworks provides tools to crosscompile and package your beat for different platforms. This requires [docker](https://www.docker.com/) and vendoring as described above. To build packages of the rt beat, run the following command:

```
make package
```

This will fetch and create all images required for the build process. The whole process to finish can take several minutes.

### Building and Releasing

**rtBeat** uses [GORELEASER] to build binaries and [Docker] containers.

#### Test Release Steps

Install [GORELEASER] with [brew] (MacOS):
```bash
brew install goreleaser/tap/goreleaser
```

Build without releasing:
```bash
goreleaser --skip-publish --rm-dist --skip-validate
```

#### Release Steps

- Commit latest changes
- [Tag] a version `git tag -a v1.0 -m "Version 1.0"`
- Push tag `git push origin v2.0`
- Run: `GITHUB_TOKEN=$GITHUB_TOKEN goreleaser --rm-dist`

## Resources

- [GORELEASER]
- [Docker]
- [homebrew]
- [elasticsearch]
- [logstash]
- [kafka]
- [redis]


[elasticsearch]: https://www.elastic.co/
[logstash]: https://www.elastic.co/products/logstash
[kafka]: https://kafka.apache.org/
[redis]: https://redis.io/
[homebrew]: https://brew.sh/
[brew]: https://brew.sh/
[GORELEASER]: https://goreleaser.com/
[Docker]: https://www.docker.com/
[Tag]: https://git-scm.com/book/en/v2/Git-Basics-Tagging
