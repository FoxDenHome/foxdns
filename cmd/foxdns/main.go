package main

import (
	"crypto/tls"
	"log"

	"github.com/FoxDenHome/foxdns/generator/authority"
	"github.com/FoxDenHome/foxdns/generator/localizer"
	"github.com/FoxDenHome/foxdns/generator/rdns"
	"github.com/FoxDenHome/foxdns/generator/resolver"
	"github.com/FoxDenHome/foxdns/generator/simple"
	"github.com/FoxDenHome/foxdns/generator/static"
	"github.com/FoxDenHome/foxdns/server"
	"github.com/miekg/dns"
)

func main() {
	config := LoadConfig("config.yml")

	srv := server.NewServer(config.Global.Listen)

	// Load dynamic config begin
	mux := dns.NewServeMux()

	for _, rdnsConf := range config.RDNS {
		rdnsAuth := authority.NewAuthorityHandler(append([]string{
			rdnsConf.Suffix,
		}, rdnsConf.Subnets...), config.Global.NameServers, config.Global.Mailbox)

		rdnsGen := rdns.NewRDNSGenerator(rdnsConf.IPVersion)
		if rdnsGen == nil {
			log.Panicf("Unknown IP version: %d", rdnsConf.IPVersion)
		}
		rdnsGen.PTRSuffix = rdnsConf.Suffix

		rdnsAuth.Child = rdnsGen

		rdnsAuth.Register(mux)
	}

	for _, resolvConf := range config.Resolvers {
		resolv := resolver.New(resolvConf.NameServers)
		if resolvConf.ServerName != "" {
			resolv.Client.TLSConfig = &tls.Config{
				ServerName: resolvConf.ServerName,
			}
		}

		if len(resolvConf.Proto) > 0 {
			resolv.Client.Net = resolvConf.Proto
		}

		resolv.AllowOnlyFromPrivate = resolvConf.AllowOnlyFromPrivate

		if resolvConf.MaxConnections > 0 {
			resolv.MaxConnections = resolvConf.MaxConnections
		}

		if resolvConf.Retries > 0 {
			resolv.Retries = resolvConf.Retries
		}

		if resolvConf.RetryWait > 0 {
			resolv.RetryWait = resolvConf.RetryWait
		}

		if resolvConf.Timeout > 0 {
			resolv.SetTimeout(resolvConf.Timeout)
		}

		mux.Handle(resolvConf.Zone, resolv)

		log.Printf("Resolver enabled for zone %s (only private clients: %v)", resolvConf.Zone, resolv.AllowOnlyFromPrivate)
	}

	if len(config.Localizers) > 0 {
		loc := localizer.New()
		locZones := make([]string, 0, len(config.Localizers))

		for locName, locIPs := range config.Localizers {
			locZones = append(locZones, locName)
			for _, ip := range locIPs {
				loc.AddRecord(locName, ip)
			}
		}

		locAuth := simple.New(locZones)
		locAuth.Child = loc
		locAuth.Register(mux)

		log.Printf("Localizer enabled for %d zones", len(locZones))
	}

	if len(config.StaticZones) > 0 {
		stat := static.New()
		statZones := make([]string, 0, len(config.StaticZones))

		for statName, statFile := range config.StaticZones {
			statZones = append(statZones, statName)
			err := stat.LoadZoneFile(statFile, statName, 3600, false)
			if err != nil {
				log.Printf("Error loading static zone %s: %v", statName, err)
			}
		}

		locAuth := simple.New(statZones)
		locAuth.Child = stat
		locAuth.Register(mux)

		log.Printf("Static zones enabled for %d zones", len(statZones))
	}

	srv.SetHandler(mux)
	// Load dynamic config end

	srv.Serve()
}
