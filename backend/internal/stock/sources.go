package stock

import "context"

// Sources searches Pexels videos first when a key is set, and falls back to
// Wikimedia Commons photos (no key needed) when Pexels is unavailable or
// finds nothing for a query.
type Sources struct {
	Pexels  *Client
	Commons *Commons
}

// Available reports whether any source can be used.
func (s Sources) Available() bool { return s.Pexels.Available() || s.Commons.Available() }

// VideoAvailable reports whether video clips (Pexels) can be used.
func (s Sources) VideoAvailable() bool { return s.Pexels.Available() }

// Search returns clips for query from the first source that has some.
func (s Sources) Search(ctx context.Context, query, orientation string, width, height, perPage int) ([]Clip, error) {
	var pexelsErr error
	if s.Pexels.Available() {
		clips, err := s.Pexels.Search(ctx, query, orientation, width, height, perPage)
		if err == nil && len(clips) > 0 {
			return clips, nil
		}
		pexelsErr = err
	}
	if s.Commons.Available() {
		return s.Commons.Search(ctx, query, orientation, width, height, perPage)
	}
	if pexelsErr != nil {
		return nil, pexelsErr
	}
	if !s.Pexels.Available() {
		return nil, ErrNoKey
	}
	return nil, nil
}

// SearchImages returns photos only (for pop-up images), from Commons.
func (s Sources) SearchImages(ctx context.Context, query string, width, height, perPage int) ([]Clip, error) {
	if !s.Commons.Available() {
		return nil, nil
	}
	return s.Commons.Search(ctx, query, "", width, height, perPage)
}

// Download fetches a clip from the source it came from.
func (s Sources) Download(ctx context.Context, clip Clip, dir string) (string, error) {
	if clip.Provider == ProviderCommons {
		return s.Commons.Download(ctx, clip, dir)
	}
	return s.Pexels.Download(ctx, clip, dir)
}
