package handlers

import (
	"aurarisk-backend/internal/services"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
)

func GetRisk(c *gin.Context) {
	latStr := c.Query("lat")
	lonStr := c.Query("lon")

	if latStr == "" || lonStr == "" {
		respondError(c, http.StatusBadRequest, "lat and lon are required")
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid lat")
		return
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid lon")
		return
	}

	locationName := "Selected Location"

	risk, err := services.GenerateRiskAssessment(c.Request.Context(), lat, lon, locationName)
	if errors.Is(err, services.ErrWeatherUnavailable) {
		c.Header("Retry-After", "60")
		respondError(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, risk)

}

type clientWeatherRiskRequest struct {
	Lat     *float64                    `json:"lat"`
	Lon     *float64                    `json:"lon"`
	Weather *services.OpenMeteoResponse `json:"weather"`
}

// PostRiskWithWeather scores weather the client fetched from Open-Meteo
// itself. Clients use it after GET /api/risk answers 503, so a server whose
// own Open-Meteo calls are refused can still serve risk.
func PostRiskWithWeather(c *gin.Context) {
	var req clientWeatherRiskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestBody(c, err)
		return
	}
	if req.Lat == nil || *req.Lat < -90 || *req.Lat > 90 {
		respondError(c, http.StatusBadRequest, "lat must be between -90 and 90")
		return
	}
	if req.Lon == nil || *req.Lon < -180 || *req.Lon > 180 {
		respondError(c, http.StatusBadRequest, "lon must be between -180 and 180")
		return
	}
	now := time.Now()
	if err := services.ValidateClientWeather(req.Weather, *req.Lat, *req.Lon, now); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, services.AssessClientWeather(c.Request.Context(), *req.Lat, *req.Lon, "Selected Location", req.Weather, now))
}
