package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Variables globales
var courseCollection *mongo.Collection
var synonyms []string // Lista donde guardamos palabras clave relevantes (categoría, título, descripción).

// Cargar variables de entorno
func loadEnv() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

// Conectar a MongoDB
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

// Recopilar todos los cursos y generar los sinónimos
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
			log.Fatalf("Error decoding course: %v", err)
		}
		courses = append(courses, course)

		// Extraer y almacenar las categorías, títulos y descripciones en la lista de sinónimos
		category := course["category"].(string)
		title := course["title"].(string)
		description := course["description"].(string)

		// Añadir a la lista de sinónimos solo palabras clave importantes
		addIfUnique(category)
		extractAndAddKeywords(title)
		extractAndAddKeywords(description)
	}

	// Imprimir los sinónimos recopilados
	log.Printf("Synonyms collected: %v", synonyms)
}

// Añadir a la lista de sinónimos si no existe ya
func addIfUnique(synonym string) {
	synonym = strings.ToLower(synonym)
	for _, existingSynonym := range synonyms {
		if existingSynonym == synonym {
			return
		}
	}
	synonyms = append(synonyms, synonym)
}

// Extraer palabras clave importantes del título o descripción
func extractAndAddKeywords(text string) {
	// Simulamos un análisis básico de palabras clave, podría mejorarse con un analizador léxico más avanzado
	keywords := []string{"curso", "desarrollo", "web", "programación", "móviles", "python", "javascript", "frontend", "backend"}

	// Dividimos el texto en palabras
	words := strings.Fields(strings.ToLower(text))

	for _, word := range words {
		// Si la palabra es clave, la añadimos a los sinónimos
		for _, keyword := range keywords {
			if strings.Contains(word, keyword) {
				addIfUnique(word)
			}
		}
	}
}

// Función para consultar cursos en la base de datos por categoría, título o descripción
func getCoursesByCategory(category string) ([]map[string]interface{}, error) {
	var courses []map[string]interface{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	orConditions := []bson.M{
		{"category": bson.M{"$regex": category, "$options": "i"}},
		{"title": bson.M{"$regex": category, "$options": "i"}},
		{"description": bson.M{"$regex": category, "$options": "i"}},
	}

	filter := bson.M{"$or": orConditions}

	cursor, err := courseCollection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving courses: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var course map[string]interface{}
		if err := cursor.Decode(&course); err != nil {
			return nil, fmt.Errorf("Error decoding course: %v", err)
		}
		courses = append(courses, course)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("Cursor error: %v", err)
	}

	// Imprime el resultado en los logs
	log.Printf("Courses found: %d\n", len(courses))

	return courses, nil
}

// Función para manejar la consulta del usuario
func chatHandler(w http.ResponseWriter, r *http.Request) {
	question := r.URL.Query().Get("question")
	if question == "" {
		http.Error(w, "Missing question parameter", http.StatusBadRequest)
		return
	}

	// Verificar si la pregunta contiene alguna de las palabras clave almacenadas
	for _, synonym := range synonyms {
		if strings.Contains(strings.ToLower(question), synonym) {
			// Encontrar y devolver los cursos relacionados con el sinónimo encontrado
			courses, err := getCoursesByCategory(synonym)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if len(courses) == 0 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"response": "No se encontraron cursos en esta categoría."})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(courses)
			return
		}
	}

	// Si no se detecta nada relevante en la consulta, hacer un manejo genérico
	handleCourseRequest(w, question)
}

func handleCourseRequest(w http.ResponseWriter, question string) {
	// Manejo genérico, por ejemplo, ordenando por precio o fecha.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"response": "No se encontraron cursos relacionados con tu búsqueda."})
}

// Función para verificar si la pregunta contiene palabras clave
func containsKeywords(question string, keywords []string) bool {
	for _, keyword := range keywords {
		if contains := strings.Contains(strings.ToLower(question), keyword); contains {
			return true
		}
	}
	return false
}

// Endpoint de verificación de salud del servicio
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "ChatGPT Bot Service is running!")
}

func main() {
	loadEnv()
	connectToMongoDB()

	// Cargar todos los cursos y sinónimos al iniciar la aplicación
	loadCoursesAndSynonyms()

	http.HandleFunc("/health", healthCheckHandler)
	http.HandleFunc("/chat", chatHandler)

	fmt.Println("ChatGPT Bot Service running on port 8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
