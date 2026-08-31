package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"
	"aurarisk-backend/internal/models"
	"github.com/gin-gonic/gin"
)
var db *sql.DB
func SetDatabase(database *sql.DB) {
	db = database
}
type CreateReportRequest struct {
	Location struct {
		Name string  json:"name"
		Lat  float64 json:"lat"
		Lon  float64 json:"lon"
	} json:"location"
	Category string json:"category"
	Note     string json:"note"
}
func GetReports(c *gin.Context) {
	latStr := c.Query("lat")
	lonStr := c.Query("lon")
	radiusStr := c.DefaultQuery("radius", "10")

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

radius, err := strconv.ParseFloat(radiusStr, 64)
if err != nil {
	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid radius"})
	return
}

latDelta := radius / 111.0
lonDelta := radius / (111.0 * 0.9)

query := `
	SELECT id, location_name, lat, lon, category, note, created_at
	FROM community_reports
	WHERE lat BETWEEN $1 AND $2
	AND lon BETWEEN $3 AND $4
	ORDER BY created_at DESC
	LIMIT 50
`

rows, err := db.Query(
	query,
	lat-latDelta, lat+latDelta,
	lon-lonDelta, lon+lonDelta,
)
if err != nil {
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	return
}
defer rows.Close()

var reports []models.CommunityReport
for rows.Next() {
	var r models.CommunityReport
	if err := rows.Scan(
		&r.ID,
		&r.Location.Name,
		&r.Location.Lat,
		&r.Location.Lon,
		&r.Category,
		&r.Note,
		&r.Timestamp,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	reports = append(reports, r)
}

if reports == nil {
	reports = []models.CommunityReport{}
}

c.JSON(http.StatusOK, reports)

}

func CreateReport(c *gin.Context) {
	var req CreateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
}	

if req.Location.Name == "" || req.Category == "" || req.Note == "" {
	c.JSON(http.StatusBadRequest, gin.H{"error": "location, category, and note are required"})
	return
}

validCategories := map[string]bool{
	"flooding":       true,
	"road_blocked":   true,
	"water_rising":   true,
	"drainage_issue": true,
}

if !validCategories[req.Category] {
	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category"})
	return
}

query := `
	INSERT INTO community_reports (location_name, lat, lon, category, note)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING id, created_at
`

var id string
var createdAt time.Time
err := db.QueryRow(
	query,
	req.Location.Name,
	req.Location.Lat,
	req.Location.Lon,
	req.Category,
	req.Note,
).Scan(&id, &createdAt)

if err != nil {
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	return
}

report := models.CommunityReport{
	ID:        id,
	Location:  req.Location,
	Category:  req.Category,
	Note:      req.Note,
	Timestamp: createdAt,
}

c.JSON(http.StatusCreated, report)
