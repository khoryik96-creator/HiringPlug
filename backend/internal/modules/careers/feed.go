package careers

import (
	"bytes"
	"encoding/xml"
	"net/url"
	"time"
)

// The widely used job feed format (a <source> of <job>s), read by Indeed and
// many aggregators. Text is escaped by encoding/xml.
type feed struct {
	XMLName       xml.Name  `xml:"source"`
	Publisher     string    `xml:"publisher"`
	PublisherURL  string    `xml:"publisherurl"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Jobs          []feedJob `xml:"job"`
}

type feedJob struct {
	Title           string `xml:"title"`
	Date            string `xml:"date"`
	ReferenceNumber string `xml:"referencenumber"`
	URL             string `xml:"url"`
	Company         string `xml:"company"`
	City            string `xml:"city,omitempty"`
	Description     string `xml:"description"`
	Salary          string `xml:"salary,omitempty"`
	JobType         string `xml:"jobtype,omitempty"`
	Expires         string `xml:"expirationdate,omitempty"`
}

var feedJobTypes = map[string]string{
	"full_time": "fulltime", "part_time": "parttime", "contract": "contract", "temporary": "temporary", "internship": "internship",
}

// Feed renders a careers page's ads as an XML job feed. Each job links to its
// ad on the careers page, tagged ?via=indeed: feeds are mostly read by Indeed,
// and the tag counts applicants from feeds apart from the careers page's own.
func Feed(company, slug, publicURL string, ads []Ad, now time.Time) ([]byte, error) {
	page := publicURL + "/careers/" + url.PathEscape(slug)
	f := feed{Publisher: company, PublisherURL: page, LastBuildDate: now.UTC().Format(time.RFC1123Z)}
	for _, a := range ads {
		j := feedJob{
			Title: a.Title, Date: a.PublishedAt.UTC().Format(time.RFC1123Z), ReferenceNumber: a.ID,
			URL: page + "/" + a.ID + "?via=indeed", Company: company, Description: a.Description,
		}
		if a.Location != nil {
			j.City = *a.Location
		}
		if a.SalaryText != nil {
			j.Salary = *a.SalaryText
		}
		if a.WorkType != nil {
			j.JobType = feedJobTypes[*a.WorkType]
		}
		if a.ClosesOn != nil {
			j.Expires = a.ClosesOn.Format(time.DateOnly)
		}
		f.Jobs = append(f.Jobs, j)
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
