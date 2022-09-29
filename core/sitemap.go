package core

import (
	"fmt"
	"net/url"
	"sync"

	"github.com/gocolly/colly/v2"
	sitemap "github.com/oxffaa/gopher-parse-sitemap"
	"github.com/sirupsen/logrus"
)

func ParseSiteMap(site *url.URL, crawler *Crawler, c *colly.Collector, wg *sync.WaitGroup, logObject *logrus.Logger) {
	defer wg.Done()
	sitemapUrls := []string{"/sitemap.xml", "/sitemap_news.xml", "/sitemap_index.xml", "/sitemap-index.xml", "/sitemapindex.xml",
		"/sitemap-news.xml", "/post-sitemap.xml", "/page-sitemap.xml", "/portfolio-sitemap.xml", "/home_slider-sitemap.xml", "/category-sitemap.xml",
		"/author-sitemap.xml"}

	for _, path := range sitemapUrls {
		// Ignore error when that not valid sitemap.xml path
		logObject.Infof("Trying to find %s", site.String()+path)
		_ = sitemap.ParseFromSite(site.String()+path, func(entry sitemap.Entry) error {
			outputFormat := fmt.Sprintf("[sitemap] - %s", entry.GetLocation())
			if crawler.Quiet {
				outputFormat = entry.GetLocation()
			}
			if crawler.Output != nil {
				crawler.Output.WriteToFile(outputFormat)
			}
			_ = c.Visit(entry.GetLocation())
			return nil
		})
	}

}
