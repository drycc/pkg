# Drycc Pkg

[![Build Status](https://travis-ci.org/drycc/pkg.svg?branch=main)](https://travis-ci.org/drycc/pkg)
[![codecov](https://codecov.io/gh/drycc/pkg/branch/main/graph/badge.svg)](https://codecov.io/gh/drycc/pkg)
[![Go Report Card](https://goreportcard.com/badge/github.com/drycc/pkg)](https://goreportcard.com/report/github.com/drycc/pkg)
[![GoDoc](https://godoc.org/github.com/drycc/pkg?status.svg)](https://godoc.org/github.com/drycc/pkg)

The Drycc Pkg project contains shared Go libraries that are used by
several Drycc projects.

## Usage

Add this project to your `vendor/` directory using
[dep](https://github.com/golang/dep):

```
$ dep ensure -add github.com/drycc/pkg add a dependency to the project
```

(The `-add` flag will add a dependency to the project.)
