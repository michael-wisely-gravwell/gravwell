/*************************************************************************
 * Copyright 2017 Gravwell, Inc. All rights reserved.
 * Contact: <legal@gravwell.io>
 *
 * This software may be modified and distributed under the terms of the
 * BSD 2-clause license. See the LICENSE file for details.
 **************************************************************************/

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/gravwell/timegrinder"
)

func lineConnHandlerTCP(c net.Conn, cfg handlerConfig) {
	cfg.wg.Add(1)
	id := addConn(c)
	defer cfg.wg.Done()
	defer delConn(id)
	defer c.Close()
	var rip net.IP

	if cfg.src == nil {
		ipstr, _, err := net.SplitHostPort(c.RemoteAddr().String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get host from rmote addr \"%s\": %v\n", c.RemoteAddr().String(), err)
			return
		}
		if rip = net.ParseIP(ipstr); rip == nil {
			fmt.Fprintf(os.Stderr, "Failed to get remote addr from \"%s\"\n", ipstr)
			return
		}
	} else {
		rip = cfg.src
	}

	var tg *timegrinder.TimeGrinder
	if !cfg.ignoreTimestamps {
		var err error
		tcfg := timegrinder.Config{
			EnableLeftMostSeed: true,
			FormatOverride:     cfg.formatOverride,
		}
		tg, err = timegrinder.NewTimeGrinder(tcfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get a handle on the timegrinder: %v\n", err)
			return
		}
		if cfg.setLocalTime {
			tg.SetLocalTime()
		}

	}
	bio := bufio.NewReader(c)
	for {
		data, err := bio.ReadBytes('\n')
		data = bytes.Trim(data, "\n\r\t ")

		if len(data) > 0 {
			if err := handleLog(data, rip, cfg.ignoreTimestamps, cfg.tag, cfg.ch, tg); err != nil {
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				lerr, ok := err.(*net.OpError)
				if !ok || lerr.Temporary() {
					fmt.Fprintf(os.Stderr, "Failed to read line: %v\n", err)
				}
			}
			return
		}

	}
}

func lineConnHandlerUDP(c *net.UDPConn, cfg handlerConfig) {
	sp := []byte("\n")
	buff := make([]byte, 16*1024) //local buffer that should be big enough for even the largest UDP packets
	tcfg := timegrinder.Config{
		EnableLeftMostSeed: true,
		FormatOverride:     cfg.formatOverride,
	}
	tg, err := timegrinder.NewTimeGrinder(tcfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get a handle on the timegrinder: %v\n", err)
		return
	}
	if cfg.setLocalTime {
		tg.SetLocalTime()
	}

	for {
		var rip net.IP
		n, raddr, err := c.ReadFromUDP(buff)
		if err != nil {
			break
		}
		if n == 0 {
			continue
		}
		if raddr == nil {
			continue
		}
		if n > len(buff) {
			continue
		}
		if cfg.src == nil {
			rip = raddr.IP
		} else {
			rip = cfg.src
		}

		lns := bytes.Split(buff[:n], sp)
		for _, ln := range lns {
			ln = bytes.Trim(ln, "\n\r\t ")
			if len(ln) == 0 {
				continue
			}
			//because we are using and reusing a local buffer, we have to copy the bytes when handing in
			if err := handleLog(append([]byte(nil), ln...), rip, cfg.ignoreTimestamps, cfg.tag, cfg.ch, tg); err != nil {
				return
			}
		}
	}

}
