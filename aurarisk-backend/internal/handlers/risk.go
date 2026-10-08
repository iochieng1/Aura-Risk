package handlers

import (
	"aurarisk-backend/internal/services"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

func GetRisk(c *gin.Context) {
	latStr := c.Query("lat")
	lonStr := c.Query("lon")

	if latStr == "" || lonStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "lat and lon are required"})
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lat"})
		return
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lon"})
		return
	}

	locationName := "Selected Location"

	risk, err := services.GenerateRiskAssessment(c.Request.Context(), lat, lon, locationName)
	if errors.Is(err, services.ErrWeatherUnavailable) {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, risk)

}
