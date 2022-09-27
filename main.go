package main

import (
	"bufio"
	"fmt"
	"gospider/config"
	"gospider/core"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	jsoniter "github.com/json-iterator/go"
	"github.com/sirupsen/logrus"
)

func main() {
	runSpider()
}

func runSpider() {
	yaml_file, err := filepath.Abs("config/spider.yml")
	if err != nil {
		logrus.Error(err)
		return
	}

	cfg, err := config.LoadSpiderCfg(yaml_file)
	if err != nil {
		logrus.Error(err)
	}

	isDebug := cfg.Debug
	if isDebug {
		core.Logger.SetLevel(logrus.DebugLevel)
	} else {
		core.Logger.SetLevel(logrus.InfoLevel)
	}

	verbose := cfg.Verbose
	if !verbose && !isDebug {
		core.Logger.SetOutput(ioutil.Discard)
	}
	outputFolder := cfg.Output
	if outputFolder != "" {
		if _, err := os.Stat(outputFolder); os.IsNotExist(err) {
			_ = os.Mkdir(outputFolder, os.ModePerm)
		}
	}

	// Parse sites input
	var siteList []string
	siteInput := cfg.Site
	if siteInput != "" {
		siteList = append(siteList, siteInput)
	}

	sitesListInput := cfg.Sites
	if sitesListInput != "" {
		// parse from stdin
		sitesFile := core.ReadingLines(sitesListInput)
		if len(sitesFile) > 0 {
			siteList = append(siteList, sitesFile...)
		}
	}

	stat, _ := os.Stdin.Stat()
	// detect if anything came from std
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			target := strings.TrimSpace(sc.Text())
			if err := sc.Err(); err == nil && target != "" {
				siteList = append(siteList, target)
			}
		}
	}

	// Check again to make sure at least one site in slice
	if len(siteList) == 0 {
		logrus.Fatal("No site in list. Please check your site input again")
	}

	threads := cfg.Threads
	sitemap := cfg.Sitemap
	linkfinder := cfg.Js
	robots := cfg.Robots
	otherSource := cfg.OtherSource
	includeSubs := cfg.IncludeSubs
	includeOtherSourceResult := cfg.IncludeOtherSource

	base := cfg.Base

	// disable all options above
	if base {
		linkfinder = false
		robots = false
		otherSource = false
		includeSubs = false
		includeOtherSourceResult = false
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
					logrus.Errorf("Failed to parse %s: %s", rawSite, err)
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

							outputFormat := fmt.Sprintf("[other-sources] - %s", url)
							if includeOtherSourceResult {
								if crawler.JsonOutput {
									sout := core.SpiderOutput{
										Input:      crawler.Input,
										Source:     "other-sources",
										OutputType: "url",
										Output:     url,
									}
									if data, err := jsoniter.MarshalToString(sout); err == nil {
										outputFormat = data
									}
								} else if crawler.Quiet {
									outputFormat = url
								}
								fmt.Println(outputFormat)

								if crawler.Output != nil {
									crawler.Output.WriteToFile(outputFormat)
								}
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
	logrus.Info("Spidering has been finished.")
}
