// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parser

import (
	"errors"
	"testing"
)

func TestCheckContents(t *testing.T) {
	t.Parallel()

	tt := []struct {
		identifier string
		err        error
	}{
		{
			identifier: "foo",
			err:        ErrInvalidContents,
		},
		{
			identifier: "asjdkajdklajsdklajslkd27137912kskdfoo",
			err:        ErrInvalidContents,
		},
		{
			identifier: "0x66f0f6DA1852857d7789f68a28bba866671f3880Daaaaaaaaaaaaaaaaaaaaaaa",
			err:        nil,
		},
		{
			identifier: "",
			err:        ErrInvalidContents,
		},
		{
			identifier: "Ab1",
			err:        ErrInvalidContents,
		},
		{
			identifier: "ab.1",
			err:        ErrInvalidContents,
		},
		{
			identifier: "a a",
			err:        ErrInvalidContents,
		},
		{
			identifier: "a/a",
			err:        ErrInvalidContents,
		},
		{
			identifier: "😀",
			err:        ErrInvalidContents,
		},
	}
	for i, tv := range tt {
		err := CheckContents(tv.identifier)
		if !errors.Is(err, tv.err) {
			t.Fatalf("#%d: err expected %v, got %v", i, tv.err, err)
		}
	}
}
