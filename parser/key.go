// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package parser defines storage key parsing operations.
package parser

import (
	"errors"
	"regexp"
)

const (
	Delimiter          = "/"
	ByteDelimiter byte = '/'
)

var (
	ErrInvalidContents = errors.New("spaces and keys must be ^0x[a-fA-F0-9]{64}$")

	reg *regexp.Regexp
)

func init() {
	reg = regexp.MustCompile("^0x[a-fA-F0-9]{64}$")
}

// CheckContents returns an error if the identifier (space or key) format is invalid.
func CheckContents(identifier string) error {
	if !reg.MatchString(identifier) {
		return ErrInvalidContents
	}
	return nil
}
