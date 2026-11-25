package ready

import (
	"net"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("ready", setup) }

type readinessBlocksKey struct{}

type readinessBlock struct {
	config  *dnsserver.Config
	monitor monitorType
}

func setup(c *caddy.Controller) error {
	addr, monType, err := parse(c)
	if err != nil {
		return plugin.Error("ready", err)
	}

	blocks, _ := c.Get(readinessBlocksKey{}).(*[]readinessBlock)
	if blocks == nil {
		blocks = new([]readinessBlock)
		c.Set(readinessBlocksKey{}, blocks)
		registerChecks := func() error {
			var checks []readinessCheck
			for _, block := range *blocks {
				for _, p := range block.config.Handlers() {
					if r, ok := p.(Readiness); ok {
						checks = append(checks, readinessCheck{p.Name(), r, block.monitor})
					}
				}
			}
			plugins.replace(checks)
			return nil
		}
		c.OnStartup(registerChecks)
		c.OnRestartFailed(registerChecks)
	}
	*blocks = append(*blocks, readinessBlock{dnsserver.GetConfig(c), monType})

	rd := &ready{Addr: addr}
	startListener := func() error {
		uniqAddr.Set(addr, rd.onStartup)
		return uniqAddr.ForEach()
	}
	c.OnStartup(startListener)
	c.OnRestartFailed(startListener)

	c.OnRestart(rd.onFinalShutdown)
	c.OnFinalShutdown(rd.onFinalShutdown)

	return nil
}

// monitorType represents the type of monitoring behavior for the readiness plugin.
type monitorType string

const (
	// monitorTypeUntilReady indicates the monitoring should continue until the system is ready.
	monitorTypeUntilReady monitorType = "until-ready"

	// monitorTypeContinuously indicates the monitoring should continue indefinitely.
	monitorTypeContinuously monitorType = "continuously"
)

func parse(c *caddy.Controller) (string, monitorType, error) {
	addr := ":8181"
	monType := monitorTypeUntilReady

	i := 0
	for c.Next() {
		if i > 0 {
			return "", "", plugin.ErrOnce
		}
		i++
		args := c.RemainingArgs()

		switch len(args) {
		case 0:
		case 1:
			addr = args[0]
			if _, _, e := net.SplitHostPort(addr); e != nil {
				return "", "", e
			}
		default:
			return "", "", c.ArgErr()
		}

		for c.NextBlock() {
			switch c.Val() {
			case "monitor":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return "", "", c.ArgErr()
				}

				var err error
				monType, err = parseMonitorType(c, args[0])
				if err != nil {
					return "", "", err
				}
			default:
				return "", "", c.Errf("unknown property '%s'", c.Val())
			}
		}
	}
	return addr, monType, nil
}

func parseMonitorType(c *caddy.Controller, arg string) (monitorType, error) {
	switch arg {
	case "until-ready":
		return monitorTypeUntilReady, nil
	case "continuously":
		return monitorTypeContinuously, nil
	default:
		return "", c.Errf("monitor type '%s' not supported", arg)
	}
}
