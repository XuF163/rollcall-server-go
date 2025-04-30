package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"rollcall-server-go/db"     // Adjust import path
	"rollcall-server-go/models" // Adjust import path
)

// APIHandler holds the dependencies for API handlers, like the Redis service
type APIHandler struct {
	RedisService *db.RedisService
}

// NewAPIHandler creates a new APIHandler
func NewAPIHandler(service *db.RedisService) *APIHandler {
	return &APIHandler{
		RedisService: service,
	}
}

// --- Class Handlers ---

// GetAllClasses handles GET /api/classes
func (h *APIHandler) GetAllClasses(c *gin.Context) {
	classes, err := h.RedisService.GetAllClasses()
	if err != nil {
		log.Printf("Error in GetAllClasses handler: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve classes"})
		return
	}
	if classes == nil {
		// Return empty list instead of null for JSON consistency
		c.JSON(http.StatusOK, []models.Clazz{})
		return
	}
	c.JSON(http.StatusOK, classes)
}

// GetClassByID handles GET /api/classes/:classId
func (h *APIHandler) GetClassByID(c *gin.Context) {
	classID := c.Param("classId")
	if classID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Class ID is required"})
		return
	}

	clazz, err := h.RedisService.GetClassByID(classID)
	if err != nil {
		log.Printf("Error in GetClassByID handler for ID %s: %v", classID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve class details"})
		return
	}

	if clazz == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Class not found"})
		return
	}

	c.JSON(http.StatusOK, clazz)
}

// AddClass handles POST /api/classes (for testing/manual addition)
func (h *APIHandler) AddClass(c *gin.Context) {
	var newClass models.Clazz
	if err := c.ShouldBindJSON(&newClass); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	if newClass.ID == "" || newClass.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Class ID and Name are required"})
		return
	}

	err := h.RedisService.AddClass(newClass)
	if err != nil {
		log.Printf("Error in AddClass handler: %v", err)
		// Check for specific errors if needed, e.g., duplicate ID if not handled by RedisService
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add class"})
		return
	}

	c.JSON(http.StatusCreated, newClass)
}

// --- Student Handlers ---

// GetStudentsByClass handles GET /api/classes/:classId/students
func (h *APIHandler) GetStudentsByClass(c *gin.Context) {
	classID := c.Param("classId")
	if classID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Class ID is required"})
		return
	}

	// Optional: Check if class exists first
	exists, err := h.RedisService.ClassExists(classID)
	if err != nil {
		log.Printf("Error checking class existence in GetStudentsByClass handler for ID %s: %v", classID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify class"})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Class not found"})
		return
	}

	students, err := h.RedisService.GetStudentsByClassID(classID)
	if err != nil {
		log.Printf("Error in GetStudentsByClass handler for ID %s: %v", classID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve students for the class"})
		return
	}
	if students == nil {
		c.JSON(http.StatusOK, []models.Student{})
		return
	}

	c.JSON(http.StatusOK, students)
}

// GetRandomStudent handles GET /api/classes/:classId/random-student
func (h *APIHandler) GetRandomStudent(c *gin.Context) {
	classID := c.Param("classId")
	if classID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Class ID is required"})
		return
	}

	student, err := h.RedisService.GetRandomStudent(classID)
	if err != nil {
		log.Printf("Error in GetRandomStudent handler for ID %s: %v", classID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get random student"})
		return
	}

	if student == nil {
		// Could mean class not found, or class has no students.
		// Check if class exists to differentiate maybe?
		exists, _ := h.RedisService.ClassExists(classID)
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"message": "Class not found"})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"message": "No students found in this class"})
		}
		return
	}

	c.JSON(http.StatusOK, student)
}

// --- Import Handler ---

// ImportStudents handles POST /api/import/students
func (h *APIHandler) ImportStudents(c *gin.Context) {
	// Get classId from form data
	classID := c.PostForm("classId")
	if classID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Missing 'classId' in form data"})
		return
	}

	// Get file from form data
	file, header, err := c.Request.FormFile("file") // "file" is the name attribute in the form
	if err != nil {
		log.Printf("Error getting form file: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Error retrieving uploaded file: " + err.Error()})
		return
	}
	defer file.Close()

	log.Printf("Received file upload: %s for class: %s", header.Filename, classID)

	// Call the service layer to process the import
	importedCount, err := h.RedisService.ImportStudentsFromExcel(file, classID)
	if err != nil {
		log.Printf("Error importing students from file %s for class %s: %v", header.Filename, classID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to import students: " + err.Error()})
		return
	}

	// Success response
	c.JSON(http.StatusOK, gin.H{
		"message":       "Import successful",
		"importedCount": importedCount,
		"classId":       classID,
	})
}

// --- Ping Handler ---
func PingHandler(c *gin.Context) {
	// You might want to inject the redis client here too to ping it
	// For now, just a simple pong
	c.JSON(http.StatusOK, gin.H{"message": "Pong!"})
}
