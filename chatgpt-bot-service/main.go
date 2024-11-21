package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)


var courseCollection *mongo.Collection
var redisClient *redis.Client
var synonyms []string 


func loadEnv() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}


func connectToMongoDB() {
	clientOptions := options.Client().ApplyURI(os.Getenv("MONGO_URI"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	courseCollection = client.Database("coursesDB").Collection("courses")
	fmt.Println("Connected to MongoDB!")
}


func connectToRedis() {
	redisClient = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	_, err := redisClient.Ping(context.Background()).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	fmt.Println("Connected to Redis!")
}


func loadCoursesAndSynonyms() {
	var courses []map[string]interface{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := courseCollection.Find(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Error retrieving courses: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var course map[string]interface{}
		if err := cursor.Decode(&course); err != nil {
			log.Printf("Skipping course due to decode error: %v", err)
			continue
		}

		category := course["category"].(string)
		title := course["title"].(string)
		description := course["description"].(string)

		addIfUnique(category)
		extractAndAddKeywords(title)
		extractAndAddKeywords(description)
	}

	log.Printf("Synonyms collected: %v", synonyms)
}


func addIfUnique(synonym string) {
	synonym = strings.ToLower(synonym)
	for _, existing := range synonyms {
		if existing == synonym {
			return
		}
	}
	synonyms = append(synonyms, synonym)
}


func extractAndAddKeywords(text string) {
	keywords := []string{"curso", "desarrollo", "web", "programación", "móviles", "python", "javascript", "frontend", "backend"}
	words := strings.Fields(strings.ToLower(text))

	for _, word := range words {
		for _, keyword := range keywords {
			if strings.Contains(word, keyword) {
				addIfUnique(word)
			}
		}
	}
}


func cacheResponse(key string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	redisClient.Set(context.Background(), key, jsonData, 10*time.Minute) // TTL de 10 minutos
}


func getCachedResponse(key string) (interface{}, bool) {
	result, err := redisClient.Get(context.Background(), key).Result()
	if err != nil {
		return nil, false
	}
	var data interface{}
	json.Unmarshal([]byte(result), &data)
	return data, true
}


func getCoursesPaginated(filter bson.M, page, limit int) ([]map[string]interface{}, error) {
	var courses []map[string]interface{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	options := options.Find().SetSkip(int64((page - 1) * limit)).SetLimit(int64(limit))
	cursor, err := courseCollection.Find(ctx, filter, options)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var course map[string]interface{}
		cursor.Decode(&course)
		courses = append(courses, course)
	}

	return courses, nil
}


func analyzeIntent(question string) string {
	intents := map[string]string{
		"barato":   "precio ascendente",
		"caro":     "precio descendente",
		"nuevo":    "fecha descendente",
		"reciente": "fecha descendente",
		"antiguo":  "fecha ascendente",
	}

	for keyword, intent := range intents {
		if strings.Contains(strings.ToLower(question), keyword) {
			return intent
		}
	}
	return "categoría"
}


func chatHandler(w http.ResponseWriter, r *http.Request) {
	question := r.URL.Query().Get("question")
	if question == "" {
		http.Error(w, "Missing question parameter", http.StatusBadRequest)
		return
	}


	cacheKey := fmt.Sprintf("query:%s", strings.ToLower(question))
	if cached, found := getCachedResponse(cacheKey); found {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cached)
		return
	}


	intent := analyzeIntent(question)
	var courses []map[string]interface{}
	var err error

	switch intent {
	case "precio ascendente":
		courses, err = getCoursesPaginated(bson.M{}, 1, 10) // Ejemplo de paginación
	case "precio descendente":
		courses, err = getCoursesPaginated(bson.M{}, 1, 10)
	default:

		for _, synonym := range synonyms {
			if strings.Contains(strings.ToLower(question), synonym) {
				courses, err = getCoursesPaginated(bson.M{"$or": []bson.M{
					{"category": bson.M{"$regex": synonym, "$options": "i"}},
					{"title": bson.M{"$regex": synonym, "$options": "i"}},
					{"description": bson.M{"$regex": synonym, "$options": "i"}},
				}}, 1, 10)
				break
			}
		}
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cacheResponse(cacheKey, courses)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(courses)
}


func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "ChatGPT Bot Service is running!")
}

func main() {
	loadEnv()
	connectToMongoDB()
	connectToRedis()
	loadCoursesAndSynonyms()

	http.HandleFunc("/health", healthCheckHandler)
	http.HandleFunc("/chat", chatHandler)

	fmt.Println("ChatGPT Bot Service running on port 8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
