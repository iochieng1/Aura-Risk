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
	if msg := checkCoordinates(lat, lon); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	// A client that knows the place (a search result) names it; otherwise
	// the point is reverse-geocoded, or named by its coordinates.
	locationName, err := services.ValidatePlaceName(c.Query("name"))
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

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

// checkCoordinates returns an error message for a point off the globe. The
// comparisons also reject NaN, which strconv.ParseFloat accepts.
func checkCoordinates(lat, lon float64) string {
	if !(lat >= -90 && lat <= 90) {
		return "lat must be between -90 and 90"
	}
	if !(lon >= -180 && lon <= 180) {
		return "lon must be between -180 and 180"
	}
	return ""
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
	Name    string                      `json:"name"` // optional, as in GET
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
	if req.Lat == nil || req.Lon == nil {
		respondError(c, http.StatusBadRequest, "lat and lon are required")
		return
	}
	if msg := checkCoordinates(*req.Lat, *req.Lon); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}
	locationName, err := services.ValidatePlaceName(req.Name)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
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

	respondRisk(c, services.AssessClientWeather(c.Request.Context(), *req.Lat, *req.Lon, locationName, req.Weather, client, now))
}
