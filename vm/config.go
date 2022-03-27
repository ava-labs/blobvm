// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vm

import (
	"time"
)

type Config struct {
	BuildInterval    time.Duration `serialize:"true" json:"buildInterval"`
	GossipInterval   time.Duration `serialize:"true" json:"gossipInterval"`
	RegossipInterval time.Duration `serialize:"true" json:"regossipInterval"`

	MempoolSize       int `serialize:"true" json:"mempoolSize"`
	ActivityCacheSize int `serialize:"true" json:"activityCacheSize"`
}

func (c *Config) SetDefaults() {
	c.BuildInterval = 500 * time.Millisecond
	c.GossipInterval = 1 * time.Second
	c.RegossipInterval = 30 * time.Second

	c.MempoolSize = 1024
	c.ActivityCacheSize = 128
}
