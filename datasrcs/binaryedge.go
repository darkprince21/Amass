// Copyright 2017 Jeff Foley. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package datasrcs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/OWASP/Amass/v3/config"
	"github.com/OWASP/Amass/v3/eventbus"
	"github.com/OWASP/Amass/v3/net/http"
	"github.com/OWASP/Amass/v3/requests"
	"github.com/OWASP/Amass/v3/systems"
)

// BinaryEdge is the Service that handles access to the BinaryEdge data source.
type BinaryEdge struct {
	requests.BaseService

	API        *config.APIKey
	SourceType string
	sys        systems.System
}

// NewBinaryEdge returns he object initialized, but not yet started.
func NewBinaryEdge(sys systems.System) *BinaryEdge {
	be := &BinaryEdge{
		SourceType: requests.API,
		sys:        sys,
	}

	be.BaseService = *requests.NewBaseService(be, "BinaryEdge")
	return be
}

// Type implements the Service interface.
func (be *BinaryEdge) Type() string {
	return be.SourceType
}

// OnStart implements the Service interface.
func (be *BinaryEdge) OnStart() error {
	be.BaseService.OnStart()

	be.API = be.sys.Config().GetAPIKey(be.String())
	if be.API == nil || be.API.Key == "" {
		be.sys.Config().Log.Printf("%s: API key data was not provided", be.String())
	}

	be.SetRateLimit(2 * time.Second)
	return nil
}

// OnDNSRequest implements the Service interface.
func (be *BinaryEdge) OnDNSRequest(ctx context.Context, req *requests.DNSRequest) {
	if be.API == nil || be.API.Key == "" {
		return
	}

	cfg := ctx.Value(requests.ContextConfig).(*config.Config)
	bus := ctx.Value(requests.ContextEventBus).(*eventbus.EventBus)
	if cfg == nil || bus == nil {
		return
	}

	re := cfg.DomainRegex(req.Domain)
	if re == nil {
		return
	}

	url := be.restURL(req.Domain)
	headers := map[string]string{
		"X-KEY":        be.API.Key,
		"Content-Type": "application/json",
	}

	be.CheckRateLimit()
	bus.Publish(requests.SetActiveTopic, eventbus.PriorityCritical, be.String())
	bus.Publish(requests.LogTopic, eventbus.PriorityHigh,
		fmt.Sprintf("Querying %s for %s subdomains", be.String(), req.Domain))

	page, err := http.RequestWebPage(url, nil, headers, "", "")
	if err != nil {
		bus.Publish(requests.LogTopic, eventbus.PriorityHigh, fmt.Sprintf("%s: %s: %v", be.String(), url, err))
		return
	}
	// Extract the subdomain names from the REST API results
	var subs struct {
		Subdomains []string `json:"events"`
	}
	if err := json.Unmarshal([]byte(page), &subs); err != nil {
		return
	}

	for _, name := range subs.Subdomains {
		if re.MatchString(name) {
			bus.Publish(requests.NewNameTopic, eventbus.PriorityHigh, &requests.DNSRequest{
				Name:   name,
				Domain: req.Domain,
				Tag:    be.SourceType,
				Source: be.String(),
			})
		}
	}
}

func (be *BinaryEdge) restURL(domain string) string {
	return "https://api.binaryedge.io/v2/query/domains/subdomain/" + domain
}
