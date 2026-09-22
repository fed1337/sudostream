package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"sudoStream/internal/auth"
	"sudoStream/internal/catalog"

	"github.com/gin-gonic/gin"
)

const (
	catalogUnavailableMsg = "catalog unavailable"
	catalogUnauthorized   = "unauthorized"
	catalogParamShowKey   = "showKey"
)

// CatalogResponse is GET /api/catalog/:librarySlug.
type CatalogResponse = catalog.LibraryCatalog

// ShowCatalogResponse is GET /api/catalog/:librarySlug/shows/:showKey.
type ShowCatalogResponse = catalog.ShowDetail

// SeasonEpisodesResponse is GET /api/catalog/:librarySlug/shows/:showKey/seasons/:season/episodes.
type SeasonEpisodesResponse = catalog.SeasonEpisodes

// GetLibraryCatalog returns film or series catalog cards for a library.
//
//	@Summary		Library catalog
//	@Description	Film movie cards or series show cards for a typed library (paginated).
//	@Tags			media
//	@Produce		json
//	@Param			librarySlug	path		string	true	"Library slug"
//	@Param			limit		query		int		false	"Page size (default 28, max 100)"
//	@Param			offset		query		int		false	"Offset (default 0)"
//	@Success		200			{object}	catalog.LibraryCatalog
//	@Failure		404			{object}	ErrorResponse
//	@Failure		400			{object}	ErrorResponse
//	@Router			/api/catalog/{librarySlug} [get]
func (h *handler) getLibraryCatalog(c *gin.Context) {
	if h.catalog == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: catalogUnavailableMsg})

		return
	}

	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: catalogUnauthorized})

		return
	}

	response, err := h.catalog.GetLibraryCatalog(
		c.Request.Context(),
		user,
		c.Param("librarySlug"),
		parsePageOpts(c),
	)
	if err != nil {
		writeCatalogError(c, err)

		return
	}

	c.JSON(
		http.StatusOK,
		h.enrichCatalogFavorited(c, user, h.enrichCatalogWatched(c, user, response)),
	)
}

// GetShowCatalog returns season summaries for one show.
//
//	@Summary		Series show catalog
//	@Description	Season/special headers with episode counts (episodes loaded separately).
//	@Tags			media
//	@Produce		json
//	@Param			librarySlug	path		string	true	"Library slug"
//	@Param			showKey		path		string	true	"Normalized show key"
//	@Success		200			{object}	catalog.ShowDetail
//	@Failure		404			{object}	ErrorResponse
//	@Router			/api/catalog/{librarySlug}/shows/{showKey} [get]
func (h *handler) getShowCatalog(c *gin.Context) {
	if h.catalog == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: catalogUnavailableMsg})

		return
	}

	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: catalogUnauthorized})

		return
	}

	response, err := h.catalog.GetShow(
		c.Request.Context(),
		user,
		c.Param("librarySlug"),
		c.Param(catalogParamShowKey),
	)
	if err != nil {
		writeCatalogError(c, err)

		return
	}

	c.JSON(http.StatusOK, response)
}

// GetShowSeasonEpisodes returns episodes for one season.
//
//	@Summary		Series season episodes
//	@Description	Episodes for one season of a show (full season when limit omitted).
//	@Tags			media
//	@Produce		json
//	@Param			librarySlug	path		string	true	"Library slug"
//	@Param			showKey		path		string	true	"Normalized show key"
//	@Param			season		path		int		true	"Season number (0 = specials)"
//	@Param			limit		query		int		false	"Optional page size (omit for full season; max 100)"
//	@Param			offset		query		int		false	"Offset (default 0)"
//	@Success		200			{object}	catalog.SeasonEpisodes
//	@Failure		400			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Router			/api/catalog/{librarySlug}/shows/{showKey}/seasons/{season}/episodes [get]
func (h *handler) getShowSeasonEpisodes(c *gin.Context) {
	if h.catalog == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: catalogUnavailableMsg})

		return
	}

	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: catalogUnauthorized})

		return
	}

	season, err := strconv.Atoi(c.Param("season"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid season"})

		return
	}

	response, listErr := h.catalog.ListSeasonEpisodes(
		c.Request.Context(),
		user,
		c.Param("librarySlug"),
		c.Param(catalogParamShowKey),
		season,
		parseRawPageOpts(c),
	)
	if listErr != nil {
		writeCatalogError(c, listErr)

		return
	}

	c.JSON(
		http.StatusOK,
		h.enrichSeasonEpisodesFavorited(c, user, h.enrichSeasonEpisodesWatched(c, user, response)),
	)
}

func writeCatalogError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, catalog.ErrLibraryNotFound), errors.Is(err, catalog.ErrShowNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	case errors.Is(err, catalog.ErrUnsupportedType):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "catalog failed"})
	}
}

func (h *handler) enrichCatalogWatched(
	c *gin.Context,
	user auth.PublicUser,
	response catalog.LibraryCatalog,
) catalog.LibraryCatalog {
	if h.watch == nil {
		return response
	}

	paths := make([]string, 0, len(response.Movies)+len(response.Shows))
	for _, movie := range response.Movies {
		if movie.Path != "" {
			paths = append(paths, movie.Path)
		}
	}
	for _, show := range response.Shows {
		if show.PosterPath != "" {
			paths = append(paths, show.PosterPath)
		}
	}
	if len(paths) == 0 {
		return response
	}

	flags, err := h.watch.WatchedForPaths(c.Request.Context(), user.ID, paths[0], paths)
	if err != nil {
		return response
	}

	for index := range response.Movies {
		watched := flags[response.Movies[index].Path]
		response.Movies[index].Watched = &watched
	}

	return response
}

func (h *handler) enrichCatalogFavorited(
	c *gin.Context,
	user auth.PublicUser,
	response catalog.LibraryCatalog,
) catalog.LibraryCatalog {
	if h.favorite == nil {
		return response
	}

	paths := make([]string, 0, len(response.Movies))
	for _, movie := range response.Movies {
		if movie.Path != "" {
			paths = append(paths, movie.Path)
		}
	}
	if len(paths) == 0 {
		return response
	}

	flags, err := h.favorite.FavoritedForPaths(c.Request.Context(), user.ID, paths[0], paths)
	if err != nil {
		return response
	}

	for index := range response.Movies {
		favorited := flags[response.Movies[index].Path]
		response.Movies[index].Favorited = &favorited
	}

	return response
}

func (h *handler) enrichSeasonEpisodesWatched(
	c *gin.Context,
	user auth.PublicUser,
	response catalog.SeasonEpisodes,
) catalog.SeasonEpisodes {
	if h.watch == nil {
		return response
	}

	paths := make([]string, 0, len(response.Episodes))
	for _, episode := range response.Episodes {
		if episode.Path != "" {
			paths = append(paths, episode.Path)
		}
	}
	if len(paths) == 0 {
		return response
	}

	flags, err := h.watch.WatchedForPaths(c.Request.Context(), user.ID, paths[0], paths)
	if err != nil {
		return response
	}

	for index := range response.Episodes {
		watched := flags[response.Episodes[index].Path]
		response.Episodes[index].Watched = &watched
	}

	return response
}

func (h *handler) enrichSeasonEpisodesFavorited(
	c *gin.Context,
	user auth.PublicUser,
	response catalog.SeasonEpisodes,
) catalog.SeasonEpisodes {
	if h.favorite == nil {
		return response
	}

	paths := make([]string, 0, len(response.Episodes))
	for _, episode := range response.Episodes {
		if episode.Path != "" {
			paths = append(paths, episode.Path)
		}
	}
	if len(paths) == 0 {
		return response
	}

	flags, err := h.favorite.FavoritedForPaths(c.Request.Context(), user.ID, paths[0], paths)
	if err != nil {
		return response
	}

	for index := range response.Episodes {
		favorited := flags[response.Episodes[index].Path]
		response.Episodes[index].Favorited = &favorited
	}

	return response
}
