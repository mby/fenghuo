//go:build !linux

package collect

import (
	"errors"
	"time"

	"github.com/mby/fenghuo/internal/proto"
)

var errUnsupported = errors.New("fenghuo-agent only supports Linux")

type Collector struct{}

func New([]string) (*Collector, error) { return nil, errUnsupported }

func (c *Collector) Info() proto.Info { return proto.Info{} }

func (c *Collector) Collect(time.Time) (proto.Report, error) { return proto.Report{}, errUnsupported }
