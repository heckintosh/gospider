package main

import (
	"gospider/config"
	"gospider/core"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

func main() {
	logObject := logrus.New()
	runSpider(logObject)
}

func runSpider(logObject *logrus.Logger) {
	yaml_file, err := filepath.Abs("config/spider.yml")
	if err != nil {
		logrus.Error(err)
		return
	}

	cfg, err := config.LoadSpiderCfg(yaml_file)
	if err != nil {
		logObject.Error(err)
		return
	}
	// Parse sites input
	var siteList []string
	siteInput := cfg.Site
	if siteInput != "" {
		siteList = append(siteList, siteInput)
	}

	// Check again to make sure at least one site in slice
	if len(siteList) == 0 {
		logrus.Error("No site in list. Please check your site input again")
		return
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
					logObject.Errorf("Failed to parse %s: %s", rawSite, err)
					continue
				}

				var siteWg sync.WaitGroup

				crawler := core.NewCrawler(site, cfg, logObject)
				siteWg.Add(1)
				go func() {
					defer siteWg.Done()
					crawler.Start(linkfinder, logObject)
				}()

				// Brute force Sitemap path
				if sitemap {
					siteWg.Add(1)
					go core.ParseSiteMap(site, crawler, crawler.C, &siteWg, logObject)
				}

				// Find Robots.txt
				if robots {
					siteWg.Add(1)
					go core.ParseRobots(site, crawler, crawler.C, &siteWg, logObject)
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
}
