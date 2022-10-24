package core

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/gocolly/colly/v2"
)

func ParseRobots(site *url.URL, crawler *Crawler, c *colly.Collector, wg *sync.WaitGroup) {
	defer wg.Done()
	robotsURL := site.String() + "/robots.txt"

	resp, err := http.Get(robotsURL)
	if err != nil {
		return
	}
	if resp.StatusCode == 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		lines := strings.Split(string(body), "\n")

		var re = regexp.MustCompile(".*llow: ")
		for _, line := range lines {
			if strings.Contains(line, "llow: ") {
				url := re.ReplaceAllString(line, "")
				url = FixUrl(site, url)
				if url == "" {
					continue
				}
				_ = c.Visit(url)
			}
		}
	}

}
