package skipsegment

// ResolveIntro prefers a live chapter hit over a stored audio segment.
func ResolveIntro(chapterIntro *Intro, stored *Intro) *Intro {
	if chapterIntro != nil && chapterIntro.EndMs > chapterIntro.StartMs {
		return chapterIntro
	}
	if stored != nil && stored.EndMs > stored.StartMs {
		return stored
	}

	return nil
}
