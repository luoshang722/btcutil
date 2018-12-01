qtumutil
=======

[![Build Status](http://img.shields.io/travis/luoshang722/qtumutil.svg)](https://travis-ci.org/luoshang722/qtumutil) 
[![Coverage Status](http://img.shields.io/coveralls/luoshang722/qtumutil.svg)](https://coveralls.io/r/luoshang722/qtumutil?branch=master) 
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](http://img.shields.io/badge/godoc-reference-blue.svg)](http://godoc.org/github.com/luoshang722/qtumutil)

Package qtumutil provides litecoin-specific convenience functions and types.
A comprehensive suite of tests is provided to ensure proper functionality.  See
`test_coverage.txt` for the gocov coverage report.  Alternatively, if you are
running a POSIX OS, you can run the `cov_report.sh` script for a real-time
report.

This package was developed for qtumd, an alternative full-node implementation of
litecoin based on btcd, which is under active development by Conformal.
Although it was primarily written for qtumd, this package has intentionally been
designed so it can be used as a standalone package for any projects needing the
functionality provided.

## Installation and Updating

```bash
$ go get -u github.com/luoshang722/qtumutil
```

## License

Package qtumutil is licensed under the [copyfree](http://copyfree.org) ISC
License.
