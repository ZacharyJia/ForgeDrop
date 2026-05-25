package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	skillcatalog "forge-drop/skills"
)

func (s *Server) handlePublicSkills(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if name != "" {
		bundle, err := skillcatalog.ReadBundle(name)
		if err != nil {
			if errors.Is(err, skillcatalog.ErrNotFound) {
				writeError(c, http.StatusNotFound, "skill not found")
				return
			}
			writeError(c, http.StatusInternalServerError, "read skill failed")
			return
		}
		c.JSON(http.StatusOK, bundle)
		return
	}

	bundles, err := skillcatalog.ListBundles()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list skills failed")
		return
	}
	c.JSON(http.StatusOK, map[string]any{
		"skills": bundles,
	})
}
