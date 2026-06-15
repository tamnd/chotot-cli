// domain.go exposes chotot as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/chotot-cli/chotot"
//
// The init below registers it; the host then dereferences chotot:// URIs by
// routing to the operations Register installs. The same Domain also builds the
// standalone chotot binary, so the binary and a host share one source of truth.
package chotot

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the chotot driver. It carries no state.
type Domain struct{}

// Info describes the domain's scheme, owned hosts, and binary identity.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "chotot",
		Hosts:  []string{"chotot.com", "www.chotot.com", "gateway.chotot.com"},
		Identity: kit.Identity{
			Binary: "chotot",
			Short:  "Browse and search Chợ Tốt (chotot.com) classified listings",
			Long: `chotot reads public Chợ Tốt data over HTTPS, shapes it into clean
records, and prints output that pipes into the rest of your tools.
No API key or account is required.

chotot is an independent tool and is not affiliated with Chợ Tốt.`,
			Site: "https://www.chotot.com",
			Repo: "https://github.com/tamnd/chotot-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)
	app.CommandGroup("read", "Browse and search Chợ Tốt listings")

	// search: keyword search across all listings
	kit.Handle(app, kit.OpMeta{
		Name: "search", Group: "read", List: true,
		Summary: "Search listings by keyword",
		URIType: "listing",
		Args:    []kit.Arg{{Name: "query", Help: "keywords to search for"}},
	}, searchListings)

	// listing: fetch a single ad by ID
	kit.Handle(app, kit.OpMeta{
		Name: "listing", Group: "read", Single: true,
		Summary:  "Show full detail for one listing",
		URIType:  "listing", Resolver: true,
		Args: []kit.Arg{{Name: "id", Help: "listing ID or chotot.com URL"}},
	}, getListing)

	// browse: recent listings filtered by category and/or region
	kit.Handle(app, kit.OpMeta{
		Name: "browse", Group: "read", List: true,
		Summary: "Browse recent listings (optionally by category or region)",
		URIType: "listing",
	}, browseListings)

	// categories: list all categories
	kit.Handle(app, kit.OpMeta{
		Name: "categories", Group: "read", List: true,
		Summary: "List all Chợ Tốt listing categories",
		URIType: "category",
	}, listCategories)

	// regions: list all provinces/cities
	kit.Handle(app, kit.OpMeta{
		Name: "regions", Group: "read", List: true,
		Summary: "List all Vietnamese regions / provinces served by Chợ Tốt",
		URIType: "region",
	}, listRegions)
}

// newClient builds a *Client from the resolved kit config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	dc := DefaultConfig()
	if cfg.Rate > 0 {
		dc.Rate = cfg.Rate
	}
	if cfg.Retries >= 0 {
		dc.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		dc.Timeout = cfg.Timeout
	}
	if cfg.UserAgent != "" {
		dc.UserAgent = cfg.UserAgent
	}
	return NewClientWithConfig(dc), nil
}

// ---- input structs ----

type searchIn struct {
	Query  string  `kit:"arg"          help:"keywords to search for"`
	Page   int     `kit:"flag"         help:"page number (1-indexed)" default:"1"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type listingIn struct {
	ID     string  `kit:"arg"   help:"listing ID or chotot.com URL"`
	Client *Client `kit:"inject"`
}

type browseIn struct {
	Category string  `kit:"flag,name=category" help:"category name to filter by"`
	Region   string  `kit:"flag,name=region"   help:"region/province name to filter by"`
	Page     int     `kit:"flag"               help:"page number (1-indexed)" default:"1"`
	Limit    int     `kit:"flag,inherit"       help:"max results"`
	Client   *Client `kit:"inject"`
}

type noIn struct {
	Client *Client `kit:"inject"`
}

// ---- handlers ----

func searchListings(ctx context.Context, in searchIn, emit func(*Listing) error) error {
	lim := in.Limit
	if lim <= 0 {
		lim = 20
	}
	listings, err := in.Client.SearchListings(ctx, in.Query, in.Page, lim)
	if err != nil {
		return mapErr(err)
	}
	for _, l := range listings {
		if err := emit(l); err != nil {
			return err
		}
	}
	return nil
}

func getListing(ctx context.Context, in listingIn, emit func(*Listing) error) error {
	id := listingID(in.ID)
	if id == "" {
		return errs.Usage("invalid listing id or URL: %q", in.ID)
	}
	l, err := in.Client.GetListing(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	return emit(l)
}

func browseListings(ctx context.Context, in browseIn, emit func(*Listing) error) error {
	lim := in.Limit
	if lim <= 0 {
		lim = 20
	}
	catID, regionID := 0, 0

	if in.Category != "" {
		cats, err := in.Client.GetCategories(ctx)
		if err != nil {
			return mapErr(err)
		}
		catID = findCategoryID(cats, in.Category)
		if catID == 0 {
			return errs.Usage("unknown category: %q (run `chotot categories` to list)", in.Category)
		}
	}

	if in.Region != "" {
		regions, err := in.Client.GetRegions(ctx)
		if err != nil {
			return mapErr(err)
		}
		regionID = findRegionID(regions, in.Region)
		if regionID == 0 {
			return errs.Usage("unknown region: %q (run `chotot regions` to list)", in.Region)
		}
	}

	listings, err := in.Client.BrowseListings(ctx, catID, regionID, in.Page, lim)
	if err != nil {
		return mapErr(err)
	}
	for _, l := range listings {
		if err := emit(l); err != nil {
			return err
		}
	}
	return nil
}

func listCategories(ctx context.Context, in noIn, emit func(*Category) error) error {
	cats, err := in.Client.GetCategories(ctx)
	if err != nil {
		return mapErr(err)
	}
	for _, cat := range cats {
		if err := emit(cat); err != nil {
			return err
		}
	}
	return nil
}

func listRegions(ctx context.Context, in noIn, emit func(*Region) error) error {
	regions, err := in.Client.GetRegions(ctx)
	if err != nil {
		return mapErr(err)
	}
	for _, r := range regions {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// ---- Resolver (URI-native string functions, no network) ----

// Classify turns any accepted input into the canonical (uriType, id).
// It accepts a bare numeric ID, a full chotot.com listing URL, or a
// chotot:// URI path.
func (Domain) Classify(input string) (uriType, id string, err error) {
	id = listingID(input)
	if id == "" {
		return "", "", errs.Usage("unrecognized chotot reference: %q", input)
	}
	return "listing", id, nil
}

// Locate is the inverse: the canonical https URL for a (uriType, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "listing":
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return "", errs.Usage("invalid listing id: %q", id)
		}
		return listingURL(n), nil
	case "category", "region":
		return "https://" + Host + "/", nil
	default:
		return "", errs.Usage("chotot has no resource type %q", uriType)
	}
}

// ---- helpers ----

// listingID extracts a numeric listing ID string from any accepted input:
// a bare integer, a full chotot.com URL like https://www.chotot.com/12345678.htm,
// or a plain path segment.
func listingID(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	// Strip URL if present.
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		// path will be like /12345678.htm
		idx := strings.Index(input[8:], "/")
		if idx >= 0 {
			input = input[8+idx:]
		}
	}
	// Remove leading slash and .htm suffix.
	input = strings.TrimPrefix(input, "/")
	input = strings.TrimSuffix(input, ".htm")
	// Validate it is a non-empty integer string.
	if _, err := strconv.ParseInt(input, 10, 64); err == nil {
		return input
	}
	return ""
}

// findCategoryID returns the numeric category ID for a name match
// (case-insensitive). Returns 0 if not found.
func findCategoryID(cats []*Category, name string) int {
	name = strings.ToLower(name)
	for _, c := range cats {
		if strings.ToLower(c.Name) == name {
			n, _ := strconv.Atoi(c.ID)
			return n
		}
	}
	return 0
}

// findRegionID returns the numeric region ID for a name match
// (case-insensitive). Returns 0 if not found.
func findRegionID(regions []*Region, name string) int {
	name = strings.ToLower(name)
	for _, r := range regions {
		if strings.ToLower(r.Name) == name {
			n, _ := strconv.Atoi(r.ID)
			return n
		}
	}
	return 0
}

// mapErr translates library errors into kit error kinds for correct exit codes.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return errs.NoResults("not found")
	}
	return err
}
