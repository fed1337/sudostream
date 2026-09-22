package httpapi

import (
	"context"
	"net/http"
	"sudoStream/internal/auth"
	"sudoStream/internal/homeshelf"

	"github.com/gin-gonic/gin"
)

const homeShelvesUnavailable = "home shelves unavailable"

// getHomeContinue returns in-progress titles for the current user.
//
//	@Summary	List continue watching
//	@Tags		home
//	@Produce	json
//	@Param		limit	query		int	false	"Maximum items (default 28, max 100)"
//	@Param		offset	query		int	false	"Offset (default 0)"
//	@Success	200		{object}	HomeItemsResponse
//	@Failure	401		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/me/continue [get]
func (h *handler) getHomeContinue(c *gin.Context) {
	h.homeItems(c, h.homeShelf.Continue)
}

// getHomeFavorites returns the current user's newest favorites.
//
//	@Summary	List home favorites
//	@Tags		home
//	@Produce	json
//	@Param		limit	query		int	false	"Maximum items (default 28, max 100)"
//	@Param		offset	query		int	false	"Offset (default 0)"
//	@Success	200		{object}	HomeItemsResponse
//	@Failure	401		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/me/favorites [get]
func (h *handler) getHomeFavorites(c *gin.Context) {
	h.homeItems(c, h.homeShelf.Favorites)
}

// getHomeWatched returns the current user's recently watched media.
//
//	@Summary	List recently watched media
//	@Tags		home
//	@Produce	json
//	@Param		limit	query		int	false	"Maximum items (default 28, max 100)"
//	@Param		offset	query		int	false	"Offset (default 0)"
//	@Success	200		{object}	HomeItemsResponse
//	@Failure	401		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/me/watched [get]
func (h *handler) getHomeWatched(c *gin.Context) {
	h.homeItems(c, h.homeShelf.Watched)
}

// getHomeUnwatched returns unread indexed media.
//
//	@Summary	List unwatched media
//	@Tags		home
//	@Produce	json
//	@Param		limit	query		int	false	"Maximum items (default 28, max 100)"
//	@Param		offset	query		int	false	"Offset (default 0)"
//	@Success	200		{object}	HomeItemsResponse
//	@Failure	401		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/me/unwatched [get]
func (h *handler) getHomeUnwatched(c *gin.Context) {
	h.homeItems(c, h.homeShelf.Unwatched)
}

// getHomeStats returns watched progress for readable libraries.
//
//	@Summary	Get home library statistics
//	@Tags		home
//	@Produce	json
//	@Success	200	{object}	HomeStatsResponse
//	@Failure	401	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/me/stats [get]
func (h *handler) getHomeStats(c *gin.Context) {
	if h.homeShelf == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: homeShelvesUnavailable})

		return
	}
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	libraries, err := h.homeShelf.Stats(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "list home statistics failed"})

		return
	}

	c.JSON(http.StatusOK, HomeStatsResponse{Libraries: libraries})
}

// HomeItemsResponse is the JSON response for item shelves.
type HomeItemsResponse struct {
	Items  []homeshelf.ShelfItem `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

// HomeStatsResponse is the JSON response for library progress.
type HomeStatsResponse struct {
	Libraries []homeshelf.LibraryStat `json:"libraries"`
}

func (h *handler) homeItems(
	c *gin.Context,
	list func(context.Context, auth.PublicUser, int, int) (homeshelf.ItemPage, error),
) {
	if h.homeShelf == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: homeShelvesUnavailable})

		return
	}
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	opts := parsePageOpts(c)
	page, err := list(c.Request.Context(), user, opts.Limit, opts.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "list home shelf failed"})

		return
	}

	c.JSON(http.StatusOK, HomeItemsResponse{
		Items:  page.Items,
		Total:  page.Total,
		Limit:  page.Limit,
		Offset: page.Offset,
	})
}
