package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

// Función para consultar cursos en la base de datos por categoría
// Función para consultar cursos en la base de datos por categoría, título o descripción
// Función para consultar cursos en la base de datos por categoría, título o descripción
// Función para consultar cursos en la base de datos por categoría, título o descripción
func getCoursesByCategory(category string) ([]map[string]interface{}, error) {
	var courses []map[string]interface{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Palabras clave alternativas o sinónimos que podemos considerar
	synonyms := []string{"backend", "servidor", "API", "fullstack", "desarrollo"}

	// Ampliar la búsqueda a título, descripción y sinónimos
	orConditions := []bson.M{
		{"category": bson.M{"$regex": category, "$options": "i"}},
		{"title": bson.M{"$regex": category, "$options": "i"}},
		{"description": bson.M{"$regex": category, "$options": "i"}},
	}

	// Añadir sinónimos a la búsqueda
	for _, synonym := range synonyms {
		orConditions = append(orConditions, bson.M{
			"$or": []bson.M{
				{"category": bson.M{"$regex": synonym, "$options": "i"}},
				{"title": bson.M{"$regex": synonym, "$options": "i"}},
				{"description": bson.M{"$regex": synonym, "$options": "i"}},
			},
		})
	}

	filter := bson.M{"$or": orConditions}

	// Imprime el filtro en los logs para depuración
	log.Printf("Filter being used for category search: %+v\n", filter)

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

// Función para ordenar cursos por precio
func getCoursesOrderedByPrice(order string) ([]map[string]interface{}, error) {
	var courses []map[string]interface{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var sortOrder int
	if strings.Contains(order, "barato") {
		sortOrder = 1 // Ascendente
	} else {
		sortOrder = -1 // Descendente
	}

	options := options.Find().SetSort(bson.D{{"price", sortOrder}})
	cursor, err := courseCollection.Find(ctx, bson.M{}, options)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving courses by price: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var course map[string]interface{}
		if err := cursor.Decode(&course); err != nil {
			return nil, fmt.Errorf("Error decoding course: %v", err)
		}
		courses = append(courses, course)
	}

	return courses, nil
}

// Función para ordenar cursos por fecha
func getCoursesOrderedByDate(order string) ([]map[string]interface{}, error) {
	var courses []map[string]interface{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var sortOrder int
	if strings.Contains(order, "nuevo") || strings.Contains(order, "reciente") {
		sortOrder = -1 // Nuevos primero
	} else {
		sortOrder = 1 // Antiguos primero
	}

	options := options.Find().SetSort(bson.D{{"createdat", sortOrder}})
	cursor, err := courseCollection.Find(ctx, bson.M{}, options)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving courses by date: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var course map[string]interface{}
		if err := cursor.Decode(&course); err != nil {
			return nil, fmt.Errorf("Error decoding course: %v", err)
		}
		courses = append(courses, course)
	}

	return courses, nil
}

// Función para consultar la API de ChatGPT
func chatGPTQuery(question string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("API key not found")
	}

	url := "https://api.openai.com/v1/chat/completions"
	requestBody, _ := json.Marshal(map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": question},
		},
	})

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	choices := result["choices"].([]interface{})
	message := choices[0].(map[string]interface{})["message"].(map[string]interface{})["content"].(string)

	return message, nil
}

// Endpoint para manejar preguntas de los usuarios
func chatHandler(w http.ResponseWriter, r *http.Request) {
	question := r.URL.Query().Get("question")
	if question == "" {
		http.Error(w, "Missing question parameter", http.StatusBadRequest)
		return
	}

	// Si la consulta no contiene palabras clave, interpretarla con ChatGPT
	if !containsKeywords(question, []string{"curso", "recomendar", "barato", "caro", "programación"}) {
		interpretedQuestion, err := chatGPTQuery("Interpreta esta consulta y relaciona con cursos: " + question)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		question = interpretedQuestion
	}

	// Procesar la respuesta de ChatGPT para obtener términos clave y ajustar la consulta a MongoDB
	categoryFromAI := extractCategoryFromAI(question)

	// Si ChatGPT sugirió una categoría o contexto, usarlo
	if categoryFromAI != "" {
		courses, err := getCoursesByCategory(categoryFromAI)
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

	// Si no se detecta nada relevante en la consulta, hacer un manejo genérico
	handleCourseRequest(w, question)
}

// Función para extraer la categoría sugerida por ChatGPT a partir de la respuesta interpretada
func extractCategoryFromAI(question string) string {
	// Usamos palabras clave que puedan haber sido sugeridas por ChatGPT
	categories := []string{"programación", "frontend", "backend", "web", "desarrollo", "móviles"}

	for _, category := range categories {
		if strings.Contains(strings.ToLower(question), category) {
			return category
		}
	}
	return ""
}

func handleCourseRequest(w http.ResponseWriter, question string) {
	// Manejo de consultas por categoría
	if containsKeywords(question, []string{"programación", "frontend", "backend"}) {
		courses, err := getCoursesByCategory(question)
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

	// Ordenar cursos por precio
	if containsKeywords(question, []string{"barato", "caro", "precio"}) {
		courses, err := getCoursesOrderedByPrice(question)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(courses)
		return
	}

	// Ordenar cursos por fecha
	if containsKeywords(question, []string{"nuevo", "reciente", "antiguo"}) {
		courses, err := getCoursesOrderedByDate(question)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(courses)
		return
	}

	// Si la consulta no tiene relación con cursos, devuelve un mensaje de error
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

	http.HandleFunc("/health", healthCheckHandler)
	http.HandleFunc("/chat", chatHandler)

	fmt.Println("ChatGPT Bot Service running on port 8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
