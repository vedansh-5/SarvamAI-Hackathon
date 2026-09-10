// Package sdef parses macOS scripting definition (.sdef) files into a typed
// capability surface.
//
// Every scriptable Mac application ships an .sdef in its bundle describing the
// classes, properties, and commands it exposes to Apple Events. That is a
// first-party, machine-readable description of what the app can do — the
// structured interface VANS drives applications through.
//
// The authoritative grammar is /System/Library/DTDs/sdef.dtd.
package sdef

import "encoding/xml"

// xincludeNS is declared inline in every Cocoa .sdef, so encoding/xml resolves
// the prefix to this URI. Struct tags must spell it out literally, since Go
// tags cannot reference constants — this exists to document why.
const xincludeNS = "http://www.w3.org/2001/XInclude"

// Dictionary is the root of an .sdef file.
type Dictionary struct {
	XMLName  xml.Name  `xml:"dictionary"`
	Title    string    `xml:"title,attr"`
	Includes []Include `xml:"http://www.w3.org/2001/XInclude include"`
	Suites   []Suite   `xml:"suite"`
}

// Include is an xi:include directive. Cocoa apps use it to pull in the
// standard suite rather than redeclaring it.
type Include struct {
	Href     string `xml:"href,attr"`
	Xpointer string `xml:"xpointer,attr"`
}

// Suite groups related classes and commands.
type Suite struct {
	Name            string           `xml:"name,attr"`
	Code            string           `xml:"code,attr"`
	Hidden          string           `xml:"hidden,attr"`
	Classes         []Class          `xml:"class"`
	Commands        []Command        `xml:"command"`
	ClassExtensions []ClassExtension `xml:"class-extension"`
}

// Class is a scriptable object type, such as a note or a mailbox.
type Class struct {
	Name        string     `xml:"name,attr"`
	Code        string     `xml:"code,attr"`
	Hidden      string     `xml:"hidden,attr"`
	Description string     `xml:"description,attr"`
	Properties  []Property `xml:"property"`
	Elements    []Element  `xml:"element"`
}

// Property is an addressable attribute of a class.
type Property struct {
	Name        string    `xml:"name,attr"`
	Code        string    `xml:"code,attr"`
	Type        string    `xml:"type,attr"`   // may be empty; see TypeElems
	Access      string    `xml:"access,attr"` // often empty — empty means "rw"
	Hidden      string    `xml:"hidden,attr"`
	Description string    `xml:"description,attr"`
	TypeElems   []TypeRef `xml:"type"` // the other way a type is expressed
}

// TypeRef is a type expressed as a child element rather than an attribute.
type TypeRef struct {
	Type string `xml:"type,attr"`
	List string `xml:"list,attr"`
}

// Element is a collection a class contains, such as a folder's notes.
type Element struct {
	Type   string `xml:"type,attr"`
	Access string `xml:"access,attr"`
	Hidden string `xml:"hidden,attr"`
}

// Command is a verb the application responds to.
type Command struct {
	Name        string       `xml:"name,attr"`
	Code        string       `xml:"code,attr"`
	Hidden      string       `xml:"hidden,attr"`
	Description string       `xml:"description,attr"`
	DirectParam *DirectParam `xml:"direct-parameter"`
	Params      []Param      `xml:"parameter"`
	Result      *Result      `xml:"result"`
}

// DirectParam is a command's unnamed leading argument.
type DirectParam struct {
	Type        string    `xml:"type,attr"`
	Optional    string    `xml:"optional,attr"`
	Description string    `xml:"description,attr"`
	TypeElems   []TypeRef `xml:"type"`
}

// Param is a named command argument.
type Param struct {
	Name        string    `xml:"name,attr"`
	Code        string    `xml:"code,attr"`
	Type        string    `xml:"type,attr"`
	Optional    string    `xml:"optional,attr"`
	Hidden      string    `xml:"hidden,attr"`
	Description string    `xml:"description,attr"`
	TypeElems   []TypeRef `xml:"type"`
}

// Result is a command's return type.
type Result struct {
	Type      string    `xml:"type,attr"`
	TypeElems []TypeRef `xml:"type"`
}

// Access says whether an operation can change state.
type Access int

const (
	Read Access = iota
	Write
)

func (a Access) String() string {
	if a == Read {
		return "Read"
	}
	return "Write"
}

func isYes(v string) bool { return v == "yes" }

// TypeName returns the property's type from either the attribute or the child
// element. The sdef grammar allows both.
func (p Property) TypeName() string {
	if p.Type != "" {
		return p.Type
	}
	if len(p.TypeElems) > 0 {
		return p.TypeElems[0].Type
	}
	return ""
}

// ClassExtension adds properties and elements to a class declared elsewhere,
// usually one from the standard suite. Notes uses this to hang notes off the
// application object.
type ClassExtension struct {
	Extends     string     `xml:"extends,attr"`
	Hidden      string     `xml:"hidden,attr"`
	Description string     `xml:"description,attr"`
	Properties  []Property `xml:"property"`
	Elements    []Element  `xml:"element"`
}
