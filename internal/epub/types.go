package epub

import "encoding/xml"

const (
	nsDC  = "http://purl.org/dc/elements/1.1/"
	nsOPF = "http://www.idpf.org/2007/opf"
)

type PackageDocument struct {
	XMLName          xml.Name `xml:"package"`
	XMLNS            string   `xml:"xmlns,attr,omitempty"`
	XMLNSDC          string   `xml:"xmlns:dc,attr,omitempty"`
	XMLNSOPF         string   `xml:"xmlns:opf,attr,omitempty"`
	Version          string   `xml:"version,attr"`
	UniqueIdentifier string   `xml:"unique-identifier,attr,omitempty"`
	Lang             string   `xml:"http://www.w3.org/XML/1998/namespace lang,attr,omitempty"`
	Prefix           string   `xml:"prefix,attr,omitempty"`

	Metadata Metadata `xml:"metadata"`
	Manifest Manifest `xml:"manifest"`
	Spine    Spine    `xml:"spine"`
}

type Metadata struct {
	XMLName     xml.Name   `xml:"metadata"`
	Titles      []DCMeta   `xml:"http://purl.org/dc/elements/1.1/ title"`
	Creators    []DCMeta   `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Languages   []DCMeta   `xml:"http://purl.org/dc/elements/1.1/ language"`
	Identifiers []DCMeta   `xml:"http://purl.org/dc/elements/1.1/ identifier"`
	Meta        []MetaNode `xml:"meta"`
}

type DCMeta struct {
	ID     string `xml:"id,attr,omitempty"`
	Role   string `xml:"opf:role,attr,omitempty"`
	FileAs string `xml:"opf:file-as,attr,omitempty"`
	Value  string `xml:",chardata"`
}

type MetaNode struct {
	Property string `xml:"property,attr,omitempty"`
	Name     string `xml:"name,attr,omitempty"`
	Content  string `xml:"content,attr,omitempty"`
	Value    string `xml:",chardata"`
}

type Manifest struct {
	Items []ManifestItem `xml:"item"`
}

type ManifestItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr,omitempty"`
	Fallback   string `xml:"fallback,attr,omitempty"`
}

type Spine struct {
	ID                       string         `xml:"id,attr,omitempty"`
	PageProgressionDirection string         `xml:"page-progression-direction,attr,omitempty"`
	Itemrefs                 []SpineItemRef `xml:"itemref"`
}

type SpineItemRef struct {
	IDRef  string `xml:"idref,attr"`
	Linear string `xml:"linear,attr,omitempty"`
}

type containerRoot struct {
	Rootfiles []rootfile `xml:"rootfiles>rootfile"`
}

type rootfile struct {
	FullPath string `xml:"full-path,attr"`
}

type MergeOptions struct {
	OutPath  string
	Title    string
	Language string
	Creators []string
}
