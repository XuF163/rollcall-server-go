package main

import (
	"context" // <--- 添加 context 包导入
	"errors"  // <--- 添加 errors 包导入 (用于检查 redis.Nil)
	"log"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8" // <--- 添加 redis 包导入 (用于 redis.Nil 和 client 类型)
	"rollcall-server-go/db"        // 确保路径正确
	"rollcall-server-go/handlers"  // 确保路径正确
	"rollcall-server-go/models"    // <--- 添加 models 包导入
)

// 定义用于检查数据是否存在的关键 Key (应与 db 包中的定义一致)
// 为了简单起见，这里暂时重复定义，更好的做法是从 db 包导出或使用常量包
const classesKeyForCheck = "classes"

func main() {
	// Initialize Redis Client
	redisClient := db.InitializeRedisClient()

	// Create Redis Service
	redisService := db.NewRedisService(redisClient)

	// --- 检查并添加初始测试数据 ---
	checkAndSeedData(redisClient, redisService)
	// --- 结束检查和添加数据 ---

	// Create API Handler (injecting the service)
	apiHandler := handlers.NewAPIHandler(redisService)

	// Initialize Gin router
	router := gin.Default()
	// ... (可能需要 CORS 等中间件) ...

	// Setup API routes
	api := router.Group("/api")
	{
		// Class routes
		api.GET("/classes", apiHandler.GetAllClasses)
		api.GET("/classes/:classId", apiHandler.GetClassByID)
		api.POST("/classes", apiHandler.AddClass) // Added for manual/test addition

		// Student routes within a class
		api.GET("/classes/:classId/students", apiHandler.GetStudentsByClass)
		api.GET("/classes/:classId/random-student", apiHandler.GetRandomStudent)

		// Import route
		api.POST("/import/students", apiHandler.ImportStudents)

		// Ping route (moved under /api for consistency)
		api.GET("/ping", handlers.PingHandler)
	}

	// Start the server
	port := ":8080" // Or get from environment variable
	log.Printf("Starting server on port %s", port)
	if err := router.Run(port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

// checkAndSeedData 检查 Redis 中是否存在数据，如果不存在则添加测试数据
func checkAndSeedData(client *redis.Client, service *db.RedisService) {
	ctx := context.Background()
	// 使用 SCARD 检查 "classes" set 的大小
	count, err := client.SCard(ctx, classesKeyForCheck).Result()

	// 处理检查过程中的错误 (忽略 redis.Nil 错误，因为它也表示 Key 不存在)
	if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("警告: 无法检查 Redis 中是否存在数据 (Key: %s): %v。跳过添加测试数据。", classesKeyForCheck, err)
		return // 如果无法可靠检查，则不进行后续操作
	}

	// 如果 count 为 0 (表示 key 不存在或为空)，则添加测试数据
	if count == 0 {
		log.Printf("在 Redis 中未找到现有班级数据 (Key: '%s')。正在添加初始测试数据...", classesKeyForCheck)
		seedInitialData(service) // 调用添加数据的函数
	} else {
		log.Printf("在 Redis 中找到现有数据 (Key: '%s', 数量: %d)。跳过添加测试数据。", classesKeyForCheck, count)
	}
}

// seedInitialData 添加预设的测试班级和学生数据
func seedInitialData(s *db.RedisService) {
	log.Println("正在添加初始测试数据...")

	// 定义测试班级
	class1 := models.Clazz{ID: "C_TEST_CS01", Name: "测试计算机班"}
	class2 := models.Clazz{ID: "C_TEST_EE02", Name: "测试电子班"}

	// 添加测试班级 (打印错误但不中断)
	err1 := s.AddClass(class1)
	if err1 != nil {
		log.Printf("添加测试班级 %s 时出错: %v", class1.ID, err1)
	}
	err2 := s.AddClass(class2)
	if err2 != nil {
		log.Printf("添加测试班级 %s 时出错: %v", class2.ID, err2)
	}

	// 定义测试学生
	student1 := models.Student{ID: "S_TEST_CS01_001", Name: "张三 (测试)", ClassID: class1.ID}
	student2 := models.Student{ID: "S_TEST_CS01_002", Name: "李四 (测试)", ClassID: class1.ID}
	student3 := models.Student{ID: "S_TEST_EE02_001", Name: "王五 (测试)", ClassID: class2.ID}

	// 添加测试学生 (使用 _ 忽略错误，或者也进行错误处理)
	err := s.AddStudent(student1)
	if err != nil {
		log.Printf("添加测试学生 %s 时出错: %v", student1.ID, err)
	}
	err = s.AddStudent(student2)
	if err != nil {
		log.Printf("添加测试学生 %s 时出错: %v", student2.ID, err)
	}
	err = s.AddStudent(student3)
	if err != nil {
		log.Printf("添加测试学生 %s 时出错: %v", student3.ID, err)
	}

	log.Println("初始测试数据添加完成。")
}
