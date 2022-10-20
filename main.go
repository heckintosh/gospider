package main

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/heckintosh/gospider/config"
	"github.com/heckintosh/gospider/core"
)

func main() {
	runSpider()
}

func runSpider() error {
	yaml_file, err := filepath.Abs("config/spider.yml")
	if err != nil {
		return err
	}

	cfg, err := config.LoadSpiderCfg(yaml_file)
	if err != nil {
		return err
	}
	// Parse sites input
	var siteList []string
	siteInput := cfg.Site
	if siteInput != "" {
		siteList = append(siteList, siteInput)
	}

	// Check again to make sure at least one site in slice
	if len(siteList) == 0 {
		return errors.New("no site in list, check your site input again")
	}

	threads := cfg.Threads
	sitemap := cfg.Sitemap
	linkfinder := cfg.Js
	robots := cfg.Robots
	otherSource := cfg.OtherSource
	includeSubs := cfg.IncludeSubs
	base := cfg.Base

	// disable all options above
	if base {
		linkfinder = false
		robots = false
		otherSource = false
		includeSubs = false
	}

	var wg sync.WaitGroup
	inputChan := make(chan string, threads)
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rawSite := range inputChan {
				site, err := url.Parse(rawSite)
				if err != nil {
					fmt.Printf("Failed to parse %s: %s", rawSite, err)
					continue
				}

				var siteWg sync.WaitGroup

				crawler := core.NewCrawler(site, cfg)
				siteWg.Add(1)
				go func() {
					defer siteWg.Done()
					crawler.Start(linkfinder)
				}()

				// Brute force Sitemap path
				if sitemap {
					siteWg.Add(1)
					go core.ParseSiteMap(site, crawler, crawler.C, &siteWg)
				}

				// Find Robots.txt
				if robots {
					siteWg.Add(1)
					go core.ParseRobots(site, crawler, crawler.C, &siteWg)
				}

				if otherSource {
					siteWg.Add(1)
					go func() {
						defer siteWg.Done()
						urls := core.OtherSources(site.Hostname(), includeSubs)
						for _, url := range urls {
							url = strings.TrimSpace(url)
							if len(url) == 0 {
								continue
							}

							_ = crawler.C.Visit(url)
						}
					}()
				}
				siteWg.Wait()
				crawler.C.Wait()
				crawler.LinkFinderCollector.Wait()
			}
		}()
	}

	for _, site := range siteList {
		inputChan <- site
	}
	close(inputChan)
	wg.Wait()
	return nil
}
