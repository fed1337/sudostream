package mediafs

// DefaultPageLimit is the API default page size (catalog tiles / browse rows).
const DefaultPageLimit = 28

// MaxPageLimit caps limit query params.
const MaxPageLimit = 100

// PageOpts holds limit/offset query values before normalization.
type PageOpts struct {
	Limit  int
	Offset int
}

// PageMeta is returned with paginated list responses.
type PageMeta struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// NormalizePageOpts clamps limit/offset to API rules.
func NormalizePageOpts(opts PageOpts) PageOpts {
	if opts.Limit <= 0 {
		opts.Limit = DefaultPageLimit
	}
	if opts.Limit > MaxPageLimit {
		opts.Limit = MaxPageLimit
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}

	return opts
}

// SlicePage returns one page of items and page metadata for the full slice length.
func SlicePage[T any](items []T, opts PageOpts) ([]T, PageMeta) {
	opts = NormalizePageOpts(opts)
	total := len(items)
	meta := PageMeta{Total: total, Limit: opts.Limit, Offset: opts.Offset}
	if opts.Offset >= total {
		return []T{}, meta
	}
	end := min(opts.Offset+opts.Limit, total)

	return items[opts.Offset:end], meta
}

// ApplyPage replaces Folder.Children with a page and sets browse page meta.
func ApplyPage(response *BrowseResponse, opts PageOpts) {
	if response == nil {
		return
	}

	page, meta := SlicePage(response.Folder.Children, opts)
	response.Folder.Children = page
	response.Total = meta.Total
	response.Limit = meta.Limit
	response.Offset = meta.Offset
}
