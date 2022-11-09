package main

import (
	// "github.com/heckintosh/gospider/config"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/heckintosh/gospider/config"
	"github.com/heckintosh/gospider/core"
)

func main() {
	result, err := RunSpider("config/spider.yml", "http://example.com")
	if err != nil {
		fmt.Println(err)
	}
	spew.Dump(result)
}

func RunSpider(filename string, hosturl string) ([]string, error) {
	var result []string
	yaml_file, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadSpiderCfg(yaml_file)
	if err != nil {
		return nil, err
	}
	// Parse sites input
	var siteList []string
	siteInput := hosturl
	if siteInput != "" {
		siteList = append(siteList, siteInput)
	}

	// Check again to make sure at least one site in slice
	if len(siteList) == 0 {
		return nil, errors.New("no site in list, please check your site input again")
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
				result = crawler.Result
				temp := []string{}
				for _, item := range result {
					// Add to final result if the path is not in blacklist
					if !isPathInBlacklist(crawler.Blacklist, item) {
						temp = append(temp, item)
					}
				}
				result = temp
			}
		}()
	}

	for _, site := range siteList {
		inputChan <- site
	}
	close(inputChan)
	wg.Wait()
	return getUniqueSlice(result), err
}

func RemoveIndex(s []string, index int) []string {
	ret := make([]string, 0)
	ret = append(ret, s[:index]...)
	return append(ret, s[index+1:]...)
}

func isPathInBlacklist(blacklist []string, str string) bool {
	u, _ := url.Parse(str)
	// Not getting root elem
	if u.Path != "/" && len(u.Path) != 0 {
		for _, blacklist_entry := range blacklist {
			// If there is a blacklist part in u.path
			if strings.Contains(u.Path, blacklist_entry) {
				return true
			}
		}
	}
	return false
}

func getUniqueSlice(stringSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}

	// If the key(values of the slice) is not equal
	// to the already present value in new slice (list)
	// then we append it. else we jump on another element.
	for _, entry := range stringSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
