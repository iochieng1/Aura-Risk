package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
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

	respondRisk(c, risk)
}

// respondRisk sends an assessment with its summary and tips in the caller's
// language (lang query parameter, else Accept-Language; English by default).
// Only the response is localized: alerts and logs stay in English.
func respondRisk(c *gin.Context, risk *models.RiskAssessment) {
	lang := services.NegotiateLanguage(c.Query("lang"), c.GetHeader("Accept-Language"))
	services.LocalizeRisk(risk, lang)
	c.Header("Content-Language", lang)
	c.Header("Vary", "Accept-Language")
	c.JSON(http.StatusOK, risk)
}

type clientWeatherRiskRequest struct {
	Lat     *float64                    `json:"lat"`
	Lon     *float64                    `json:"lon"`
	Weather *services.OpenMeteoResponse `json:"weather"`
	// Optional Elevation and Flood API responses fetched by the client too,
	// used only where the server has no data of its own (#69).
	Elevation *services.ClientElevation `json:"elevation"`
	Flood     []services.RiverSeries    `json:"flood"`
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

	terrain, err := services.ClientTerrain(req.Elevation)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	river, err := services.ClientRiver(req.Flood, *req.Lat, *req.Lon, now)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	client := services.Hydrology{Terrain: terrain, River: river}

	respondRisk(c, services.AssessClientWeather(c.Request.Context(), *req.Lat, *req.Lon, "Selected Location", req.Weather, client, now))
}
