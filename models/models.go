package models

// Clazz represents a class
type Clazz struct {
	ID   string `json:"id"`   // Unique class ID
	Name string `json:"name"` // Class name
}

// Student represents a student
type Student struct {
	ID      string `json:"id"`      // Unique student ID (e.g., student number)
	Name    string `json:"name"`    // Student name
	ClassID string `json:"classId"` // ID of the class the student belongs to
}
