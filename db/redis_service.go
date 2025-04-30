package db

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/xuri/excelize/v2"
	"io"
	"log"
	"rollcall-server-go/models"
)

const (
	classesKey          = "classes"  // Set: Stores all class IDs
	classInfoPrefix     = "class:"   // Hash prefix: class:{id} -> stores class details
	classStudentsPrefix = "class:"   // Set prefix: class:{id}:students -> stores student IDs for a class
	studentInfoPrefix   = "student:" // Hash prefix: student:{id} -> stores student details
)

// RedisService handles operations with the Redis database
type RedisService struct {
	Client *redis.Client
	Ctx    context.Context // Base context
}

// NewRedisService creates a new RedisService instance
func NewRedisService(client *redis.Client) *RedisService {
	return &RedisService{
		Client: client,
		Ctx:    context.Background(), // Use a background context as base
	}
}

// Helper to generate class info key
func getClassInfoKey(classID string) string {
	return classInfoPrefix + classID
}

// Helper to generate class students set key
func getClassStudentsKey(classID string) string {
	return classStudentsPrefix + classID + ":students"
}

// Helper to generate student info key
func getStudentInfoKey(studentID string) string {
	return studentInfoPrefix + studentID
}

// --- Class Operations ---

// AddClass adds a new class to Redis
func (s *RedisService) AddClass(clazz models.Clazz) error {
	if clazz.ID == "" || clazz.Name == "" {
		return errors.New("class ID and Name cannot be empty")
	}
	classKey := getClassInfoKey(clazz.ID)
	pipe := s.Client.Pipeline()

	// Add class ID to the global set of classes
	pipe.SAdd(s.Ctx, classesKey, clazz.ID)
	// Store class details in a Hash
	pipe.HMSet(s.Ctx, classKey, map[string]interface{}{
		"id":   clazz.ID,
		"name": clazz.Name,
	})

	_, err := pipe.Exec(s.Ctx)
	if err != nil {
		log.Printf("Error adding class %s: %v", clazz.ID, err)
		return fmt.Errorf("failed to add class to Redis: %w", err)
	}
	log.Printf("Added class: %s (%s)", clazz.Name, clazz.ID)
	return nil
}

// GetClassByID retrieves a class by its ID
func (s *RedisService) GetClassByID(classID string) (*models.Clazz, error) {
	classKey := getClassInfoKey(classID)
	data, err := s.Client.HGetAll(s.Ctx, classKey).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Not found is not necessarily an error in API context
		}
		log.Printf("Error getting class %s: %v", classID, err)
		return nil, fmt.Errorf("failed to get class from Redis: %w", err)
	}
	if len(data) == 0 {
		return nil, nil // Not found
	}

	return &models.Clazz{
		ID:   data["id"],
		Name: data["name"],
	}, nil
}

// GetAllClasses retrieves all classes
func (s *RedisService) GetAllClasses() ([]models.Clazz, error) {
	classIDs, err := s.Client.SMembers(s.Ctx, classesKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return []models.Clazz{}, nil // No classes found
		}
		log.Printf("Error getting all class IDs: %v", err)
		return nil, fmt.Errorf("failed to get class IDs from Redis: %w", err)
	}

	classes := make([]models.Clazz, 0, len(classIDs))
	for _, id := range classIDs {
		clazz, err := s.GetClassByID(id)
		if err != nil {
			// Log the error but continue trying to fetch others
			log.Printf("Error fetching details for class %s: %v", id, err)
			continue
		}
		if clazz != nil {
			classes = append(classes, *clazz)
		}
	}
	return classes, nil
}

// ClassExists checks if a class ID exists in the classes set
func (s *RedisService) ClassExists(classID string) (bool, error) {
	exists, err := s.Client.SIsMember(s.Ctx, classesKey, classID).Result()
	if err != nil {
		log.Printf("Error checking existence for class %s: %v", classID, err)
		return false, fmt.Errorf("failed to check class existence: %w", err)
	}
	return exists, nil
}

// --- Student Operations ---

// AddStudent adds a student to a class
func (s *RedisService) AddStudent(student models.Student) error {
	if student.ID == "" || student.Name == "" || student.ClassID == "" {
		return errors.New("student ID, Name, and ClassID cannot be empty")
	}

	// Check if class exists (optional, but good practice)
	exists, err := s.ClassExists(student.ClassID)
	if err != nil {
		return err // Error checking existence
	}
	if !exists {
		log.Printf("Warning: Adding student %s to non-existent class %s. Creating class.", student.ID, student.ClassID)
		// Optionally auto-create class
		err := s.AddClass(models.Clazz{ID: student.ClassID, Name: "Class " + student.ClassID})
		if err != nil {
			log.Printf("Failed to auto-create class %s: %v", student.ClassID, err)
			return fmt.Errorf("student's class %s does not exist and auto-creation failed: %w", student.ClassID, err)
		}
	}

	studentKey := getStudentInfoKey(student.ID)
	classStudentsKey := getClassStudentsKey(student.ClassID)

	pipe := s.Client.Pipeline()
	// Add student ID to the class's set of students
	pipe.SAdd(s.Ctx, classStudentsKey, student.ID)
	// Store student details in a Hash
	pipe.HMSet(s.Ctx, studentKey, map[string]interface{}{
		"id":      student.ID,
		"name":    student.Name,
		"classId": student.ClassID,
	})

	_, execErr := pipe.Exec(s.Ctx)
	if execErr != nil {
		log.Printf("Error adding student %s to class %s: %v", student.ID, student.ClassID, execErr)
		return fmt.Errorf("failed to add student to Redis: %w", execErr)
	}
	// log.Printf("Added student: %s (%s) to class %s", student.Name, student.ID, student.ClassID) // Can be noisy
	return nil
}

// GetStudentByID retrieves a student by their ID
func (s *RedisService) GetStudentByID(studentID string) (*models.Student, error) {
	studentKey := getStudentInfoKey(studentID)
	data, err := s.Client.HGetAll(s.Ctx, studentKey).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Not found
		}
		log.Printf("Error getting student %s: %v", studentID, err)
		return nil, fmt.Errorf("failed to get student from Redis: %w", err)
	}
	if len(data) == 0 {
		return nil, nil // Not found
	}

	return &models.Student{
		ID:      data["id"],
		Name:    data["name"],
		ClassID: data["classId"],
	}, nil
}

// GetStudentsByClassID retrieves all students for a given class ID
func (s *RedisService) GetStudentsByClassID(classID string) ([]models.Student, error) {
	classStudentsKey := getClassStudentsKey(classID)
	studentIDs, err := s.Client.SMembers(s.Ctx, classStudentsKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return []models.Student{}, nil // No students in this class
		}
		log.Printf("Error getting student IDs for class %s: %v", classID, err)
		return nil, fmt.Errorf("failed to get student IDs from Redis for class %s: %w", classID, err)
	}

	students := make([]models.Student, 0, len(studentIDs))
	for _, id := range studentIDs {
		student, err := s.GetStudentByID(id)
		if err != nil {
			log.Printf("Error fetching details for student %s in class %s: %v", id, classID, err)
			continue // Skip this student if details can't be fetched
		}
		if student != nil {
			students = append(students, *student)
		}
	}
	return students, nil
}

// GetRandomStudent selects a random student from a class
func (s *RedisService) GetRandomStudent(classID string) (*models.Student, error) {
	classStudentsKey := getClassStudentsKey(classID)

	// Use SRANDMEMBER to get one random member ID
	randomStudentID, err := s.Client.SRandMember(s.Ctx, classStudentsKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Class exists but has no students, or key doesn't exist
		}
		log.Printf("Error getting random student ID for class %s: %v", classID, err)
		return nil, fmt.Errorf("failed to get random student ID from Redis for class %s: %w", classID, err)
	}

	if randomStudentID == "" {
		return nil, nil // Set exists but is empty, SRandMember returns ""
	}

	// Fetch the details of the randomly selected student
	return s.GetStudentByID(randomStudentID)
}

// --- Excel Import ---

// ImportStudentsFromExcel reads an Excel file stream and adds students to the specified class
func (s *RedisService) ImportStudentsFromExcel(file io.Reader, classID string) (int, error) {
	// 1. Check if class exists (or create it)
	exists, err := s.ClassExists(classID)
	if err != nil {
		return 0, fmt.Errorf("failed to check class existence before import: %w", err)
	}
	if !exists {
		log.Printf("Import target class %s does not exist. Creating it.", classID)
		err := s.AddClass(models.Clazz{ID: classID, Name: "Imported Class " + classID})
		if err != nil {
			return 0, fmt.Errorf("target class %s does not exist and failed to create it: %w", classID, err)
		}
	}

	f, err := excelize.OpenReader(file)
	if err != nil {
		log.Printf("Error opening Excel reader: %v", err)
		return 0, fmt.Errorf("failed to open excel file: %w", err)
	}
	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
			log.Printf("Error closing excel file: %v", err)
		}
	}()

	// Assuming data is in the first sheet
	sheetName := f.GetSheetName(0) // Or f.GetSheetList()[0]
	if sheetName == "" {
		return 0, errors.New("excel file does not contain any sheets")
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		log.Printf("Error getting rows from sheet '%s': %v", sheetName, err)
		return 0, fmt.Errorf("failed to get rows from sheet %s: %w", sheetName, err)
	}

	importedCount := 0
	// Use a pipeline for potentially faster bulk inserts
	// pipe := s.Client.Pipeline()
	studentsToAdd := []models.Student{}

	// Start from row 1 (index 1) to skip header (assuming row 0 is header)
	for i, row := range rows {
		if i == 0 {
			continue // Skip header row
		}

		var studentID, studentName string

		// Assuming Column A (index 0) is Student ID, Column B (index 1) is Name
		if len(row) > 0 {
			studentID = row[0]
		}
		if len(row) > 1 {
			studentName = row[1]
		}

		// Basic validation
		if studentID == "" || studentName == "" {
			log.Printf("Skipping row %d due to missing ID or Name (ID: '%s', Name: '%s')", i+1, studentID, studentName)
			continue
		}

		student := models.Student{
			ID:      studentID, // Consider prefixing with classID if IDs aren't globally unique: classID + "_" + studentID
			Name:    studentName,
			ClassID: classID,
		}
		studentsToAdd = append(studentsToAdd, student)
	}

	// Add students using the service's AddStudent method (which uses pipeline internally)
	// Or, optimize further by building a larger pipeline here if AddStudent doesn't already
	log.Printf("Attempting to add %d students from Excel file to class %s", len(studentsToAdd), classID)
	for _, student := range studentsToAdd {
		err := s.AddStudent(student) // Calls the existing AddStudent logic
		if err != nil {
			log.Printf("Error adding student %s (%s) during import: %v", student.Name, student.ID, err)
			// Decide whether to stop import on first error or continue
			// return importedCount, fmt.Errorf("error adding student %s: %w", student.ID, err) // Stop on error
			continue // Continue processing other students
		}
		importedCount++
	}

	log.Printf("Successfully imported %d students into class %s", importedCount, classID)
	return importedCount, nil
}

// --- Seed Data (Optional) ---
func (s *RedisService) SeedData() {
	log.Println("Seeding initial data...")

	// Optional: Flush DB 5 before seeding (Use with caution!)
	// if err := s.Client.FlushDB(s.Ctx).Err(); err != nil {
	//  log.Fatalf("Failed to flush DB: %v", err)
	// }

	class1 := models.Clazz{ID: "C2024_GO01", Name: "2024 Go Backend Class 1"}
	class2 := models.Clazz{ID: "C2024_PY02", Name: "2024 Python Data Science 2"}

	_ = s.AddClass(class1)
	_ = s.AddClass(class2)

	_ = s.AddStudent(models.Student{ID: "S_GO01_001", Name: "Alice", ClassID: class1.ID})
	_ = s.AddStudent(models.Student{ID: "S_GO01_002", Name: "Bob", ClassID: class1.ID})
	_ = s.AddStudent(models.Student{ID: "S_GO01_003", Name: "Charlie", ClassID: class1.ID})

	_ = s.AddStudent(models.Student{ID: "S_PY02_001", Name: "David", ClassID: class2.ID})
	_ = s.AddStudent(models.Student{ID: "S_PY02_002", Name: "Eve", ClassID: class2.ID})

	log.Println("Seeding complete.")
}

// --- Utility ---

// InitializeRedisClient creates and tests a Redis client connection
func InitializeRedisClient() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379", // Redis server address
		Password: "",               // No password set
		DB:       8,                // Use database 5
	})

	// Ping Redis to check connection
	_, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}

	log.Println("Successfully connected to Redis DB 5")
	return rdb
}
