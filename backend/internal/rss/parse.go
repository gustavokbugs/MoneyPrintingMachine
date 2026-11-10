package rss

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
	"time"
)

// ErrNotAFeed means the document is neither RSS 2.0 nor Atom.
var ErrNotAFeed = errors.New("document is not an RSS or Atom feed")

// MaxItemsPerFeed bounds how many entries are kept from one fetch.
const MaxItemsPerFeed = 200

// Feed is a parsed feed.
type Feed struct {
	Title string
	Items []Item
}

// Item is one feed entry. Only metadata is kept.
type Item struct {
	GUIDHash    string
	Title       string
	Link        string
	PublishedAt time.Time
}

// innerText captures an element's raw content, including inline markup
// and CDATA, so titles like "NASA <b>Artemis</b>" keep every word.
type innerText struct {
	XML string `xml:",innerxml"`
}

type rssDoc struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title innerText `xml:"title"`
		Items []struct {
			Title   innerText `xml:"title"`
			Link    string    `xml:"link"`
			GUID    string    `xml:"guid"`
			PubDate string    `xml:"pubDate"`
			DCDate  string    `xml:"http://purl.org/dc/elements/1.1/ date"`
		} `xml:"item"`
	} `xml:"channel"`
}

type atomDoc struct {
	XMLName xml.Name  `xml:"http://www.w3.org/2005/Atom feed"`
	Title   innerText `xml:"title"`
	Entries []struct {
		Title     innerText `xml:"title"`
		ID        string    `xml:"id"`
		Published string    `xml:"published"`
		Updated   string    `xml:"updated"`
		Links     []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// cleanText strips markup and collapses whitespace; titles are plain text.
func cleanText(t innerText) string {
	s := strings.NewReplacer("<![CDATA[", "", "]]>", "").Replace(t.XML)
	// Strip markup, decode entities, then strip again for escaped HTML.
	s = tagRe.ReplaceAllString(html.UnescapeString(tagRe.ReplaceAllString(s, " ")), " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

var dateLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC3339, time.RFC3339Nano,
	"Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", "2 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04 -0700", "2006-01-02T15:04:05Z07:00", "2006-01-02",
}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func hashGUID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}

func safeLink(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		if len(s) > 2048 {
			return ""
		}
		return s
	}
	return ""
}

// Parse decodes RSS 2.0 or Atom. Entries without a usable date are
// skipped, because mention counts depend on publication time. Undated
// entries would otherwise inflate recent counts.
func Parse(data []byte) (*Feed, error) {
	dec := func(v any) error {
		d := xml.NewDecoder(bytes.NewReader(data))
		d.Strict = false
		// Non-UTF-8 feeds are read as-is rather than rejected; titles may
		// show replacement characters but dates and links still parse.
		d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
		return d.Decode(v)
	}
	var r rssDoc
	if err := dec(&r); err == nil && r.XMLName.Local == "rss" {
		feed := &Feed{Title: cleanText(r.Channel.Title)}
		for _, it := range r.Channel.Items {
			date, ok := parseDate(it.PubDate)
			if !ok {
				date, ok = parseDate(it.DCDate)
			}
			title := cleanText(it.Title)
			if !ok || title == "" {
				continue
			}
			id := strings.TrimSpace(it.GUID)
			if id == "" {
				id = strings.TrimSpace(it.Link)
			}
			feed.Items = append(feed.Items, Item{GUIDHash: hashGUID(id, title), Title: title, Link: safeLink(it.Link), PublishedAt: date})
			if len(feed.Items) == MaxItemsPerFeed {
				break
			}
		}
		return feed, nil
	}
	var a atomDoc
	if err := dec(&a); err == nil && a.XMLName.Local == "feed" {
		feed := &Feed{Title: cleanText(a.Title)}
		for _, e := range a.Entries {
			date, ok := parseDate(e.Published)
			if !ok {
				date, ok = parseDate(e.Updated)
			}
			title := cleanText(e.Title)
			if !ok || title == "" {
				continue
			}
			link := ""
			for _, l := range e.Links {
				if l.Rel == "" || l.Rel == "alternate" {
					link = safeLink(l.Href)
					break
				}
			}
			feed.Items = append(feed.Items, Item{GUIDHash: hashGUID(e.ID, title), Title: title, Link: link, PublishedAt: date})
			if len(feed.Items) == MaxItemsPerFeed {
				break
			}
		}
		return feed, nil
	}
	return nil, fmt.Errorf("%w", ErrNotAFeed)
}

// Tokens splits a query into lowercase words of at least two characters,
// ignoring common stop words. It is used for mention matching.
func Tokens(q string) []string {
	stop := map[string]bool{"the": true, "and": true, "of": true, "for": true, "in": true, "on": true, "a": true, "an": true, "to": true, "with": true}
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127)
	}) {
		if len(w) < 2 || stop[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
		if len(out) == 6 {
			break
		}
	}
	return out
}
