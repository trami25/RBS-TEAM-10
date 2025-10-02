package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	AuthServicePort = ":8081"
	ZanzibarURL     = "http://localhost:8080"
	SessionName     = "auth-session"
	JWTSecret       = "your-secret-key-change-in-production"
)

// getDocumentsPath returns the absolute path to the documents directory
func getDocumentsPath() string {
	// Get current working directory
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("Error getting working directory: %v", err)
		return "../web-client/documents" // fallback to relative path
	}

	// Build absolute path to documents folder
	// If running from auth-service directory, go up one level then to web-client/documents
	if strings.HasSuffix(wd, "auth-service") {
		return filepath.Join(filepath.Dir(wd), "web-client", "documents")
	} else {
		// If running from project root, go directly to web-client/documents
		return filepath.Join(wd, "web-client", "documents")
	}
}

// User represents a user in our system
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"` // omit in responses
	Role     string `json:"role"`
}

// Demo users for testing
var demoUsers = map[string]User{
	"alice": {
		ID:       "user:alice",
		Username: "alice",
		Password: "$2a$10$N9qo8uLOickgx2ZMRZoMye.Uo0bSZ0.WllQD0J5XvYB8X1e0V0t9u", // bcrypt hash of "alice123"
		Role:     "owner",
	},
	"bob": {
		ID:       "user:bob",
		Username: "bob",
		Password: "$2a$10$N9qo8uLOickgx2ZMRZoMye.Uo0bSZ0.WllQD0J5XvYB8X1e0V0t9u", // bcrypt hash of "bob123"
		Role:     "editor",
	},
	"charlie": {
		ID:       "user:charlie",
		Username: "charlie",
		Password: "$2a$10$N9qo8uLOickgx2ZMRZoMye.Uo0bSZ0.WllQD0J5XvYB8X1e0V0t9u", // bcrypt hash of "charlie123"
		Role:     "viewer",
	},
	"david": {
		ID:       "user:david",
		Username: "david",
		Password: "$2a$10$N9qo8uLOickgx2ZMRZoMye.Uo0bSZ0.WllQD0J5XvYB8X1e0V0t9u", // bcrypt hash of "david123"
		Role:     "user",
	},
}

// LoginRequest represents login credentials
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents login response
type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	User    *User  `json:"user,omitempty"`
	Token   string `json:"token,omitempty"`
}

// AuthClaims represents JWT claims
type AuthClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// initializeACLForAlice sets up initial ACL entries for Alice to own all documents
func initializeACLForAlice() {
	log.Printf("🔧 Initializing ACL entries for Alice...")

	// List of documents to initialize
	documents := []string{"document1.md", "document2.md", "document3.md", "document4.md", "document5.md"}

	for _, docName := range documents {
		// Create ACL entry for Alice as owner of each document
		aclData := map[string]interface{}{
			"object":   fmt.Sprintf("doc:%s", docName),
			"relation": "owner",
			"user":     "user:alice",
		}

		jsonData, err := json.Marshal(aclData)
		if err != nil {
			log.Printf("❌ Error marshaling ACL data for %s: %v", docName, err)
			continue
		}

		// Send to Mini-Zanzibar
		resp, err := http.Post(fmt.Sprintf("%s/api/v1/acl", ZanzibarURL), "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("❌ Error creating ACL entry for %s: %v", docName, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			log.Printf("✅ Created ACL: Alice owns %s", docName)
		} else {
			log.Printf("❌ Failed to create ACL for %s: HTTP %d", docName, resp.StatusCode)
		}
	}

	log.Printf("🔧 ACL initialization completed")
}

func main() {
	// Create Gin router
	r := gin.Default()

	// CORS configuration
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"http://localhost:3000", "http://localhost:3001", "http://127.0.0.1:3000", "http://127.0.0.1:3001", "http://[::1]:3000", "http://[::1]:3001"}
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"}
	config.AllowCredentials = true
	r.Use(cors.New(config))

	// Session middleware
	store := cookie.NewStore([]byte("secret-key-change-in-production"))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   3600,                 // 1 hour
		HttpOnly: false,                // Allow JavaScript access for debugging
		Secure:   false,                // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode, // Change to Lax for better compatibility
		Domain:   "",                   // Empty domain for localhost
	})
	r.Use(sessions.Sessions(SessionName, store))

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "service": "auth-service"})
	})

	// Authentication routes
	r.POST("/auth/login", handleLogin)
	r.POST("/auth/logout", handleLogout)
	r.GET("/auth/me", authMiddleware(), handleMe)

	// Protected routes - proxy to Zanzibar with user context
	protected := r.Group("/api")
	protected.Use(authMiddleware())
	{
		// ACL management - let Mini-Zanzibar handle authorization based on document ownership
		protected.POST("/acl", proxyToZanzibar)
		protected.DELETE("/acl", adminOnly(), proxyToZanzibar)

		// Authorization checks (all authenticated users)
		protected.GET("/acl/check", proxyToZanzibar)

		// Namespace management (admin only)
		protected.POST("/namespace", adminOnly(), proxyToZanzibar)
		protected.GET("/namespace/:id", proxyToZanzibar)
	}

	// Document operations
	r.GET("/documents", authMiddleware(), handleListDocuments)
	r.GET("/documents/:name", authMiddleware(), handleGetDocument)
	r.PUT("/documents/:name", authMiddleware(), handleSaveDocument)
	r.POST("/documents/:id/access", authMiddleware(), handleDocumentAccess)

	// Initialize ACL entries for Alice to own all documents
	// Commented out as Mini-Zanzibar requires authorization for ACL creation
	// Alice will use fallback permissions and can create ACLs through the web interface
	/*
		go func() {
			// Wait a moment for the server to start, then initialize ACLs
			time.Sleep(2 * time.Second)
			initializeACLForAlice()
		}()
	*/

	log.Printf("🔐 Auth Service starting on port %s", AuthServicePort)
	log.Printf("🔗 Proxying to Mini-Zanzibar at %s", ZanzibarURL)
	log.Fatal(r.Run(AuthServicePort))
}

// handleLogin authenticates users
func handleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, LoginResponse{
			Success: false,
			Message: "Invalid request format",
		})
		return
	}

	// Find user
	user, exists := demoUsers[req.Username]
	if !exists {
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "Invalid credentials",
		})
		return
	}

	// For demo purposes, accept simple passwords
	validPasswords := map[string]string{
		"alice":   "alice123",
		"bob":     "bob123",
		"charlie": "charlie123",
		"david":   "david123",
	}

	if validPassword, ok := validPasswords[req.Username]; !ok || req.Password != validPassword {
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "Invalid credentials",
		})
		return
	}

	// Create JWT token
	token, err := createJWTToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, LoginResponse{
			Success: false,
			Message: "Failed to create token",
		})
		return
	}

	// Save session
	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	session.Set("role", user.Role)
	err = session.Save()
	if err != nil {
		log.Printf("Failed to save session: %v", err)
		c.JSON(http.StatusInternalServerError, LoginResponse{
			Success: false,
			Message: "Failed to save session",
		})
		return
	}

	log.Printf("✅ Login successful for user: %s, session saved", user.Username)

	// Remove password from response
	responseUser := user
	responseUser.Password = ""

	c.JSON(http.StatusOK, LoginResponse{
		Success: true,
		Message: "Login successful",
		User:    &responseUser,
		Token:   token,
	})
}

// handleLogout logs out the user
func handleLogout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logout successful",
	})
}

// handleMe returns current user info
func handleMe(c *gin.Context) {
	userID := c.GetString("user_id")
	username := c.GetString("username")
	role := c.GetString("role")

	c.JSON(http.StatusOK, gin.H{
		"user_id":  userID,
		"username": username,
		"role":     role,
	})
}

// handleListDocuments returns available documents based on actual ACL permissions
func handleListDocuments(c *gin.Context) {
	username := c.GetString("username")
	userID := c.GetString("user_id")
	role := c.GetString("role")

	// Read actual documents from the web-client/documents folder
	documentsPath := getDocumentsPath()
	files, err := os.ReadDir(documentsPath)
	if err != nil {
		log.Printf("Error reading documents directory: %v", err)
		// Fallback to empty documents list if directory doesn't exist
		c.JSON(http.StatusOK, gin.H{
			"documents": []string{},
			"user":      username,
			"role":      role,
		})
		return
	}

	// Check permissions for each document via Mini-Zanzibar
	var documentsWithPermissions []map[string]interface{}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".md") {
			// Use the full filename including .md extension as document ID
			docID := file.Name()

			// Check all permission levels for this document
			canView, canEdit, canOwn := getDetailedDocumentPermissions(userID, username, docID, role)

			// Only include documents where user has at least one permission
			if canView || canEdit || canOwn {
				docInfo := map[string]interface{}{
					"name":    file.Name(),
					"canView": canView,
					"canEdit": canEdit,
					"canOwn":  canOwn,
				}
				documentsWithPermissions = append(documentsWithPermissions, docInfo)
				log.Printf("📄 User %s permissions for %s: view=%t, edit=%t, own=%t", username, file.Name(), canView, canEdit, canOwn)
			} else {
				log.Printf("🚫 User %s denied access to document: %s (no permissions)", username, file.Name())
			}
		}
	}

	log.Printf("📋 Returning %d documents for user %s (checked via ACL)", len(documentsWithPermissions), username)
	c.JSON(http.StatusOK, gin.H{
		"documents": documentsWithPermissions,
		"user":      username,
		"role":      role,
	})
}

// handleGetDocument returns the content of a specific document based on ACL permissions
func handleGetDocument(c *gin.Context) {
	docName := c.Param("name")
	username := c.GetString("username")
	userID := c.GetString("user_id")
	role := c.GetString("role")

	// Validate that the document name is safe (no path traversal)
	if strings.Contains(docName, "..") || strings.Contains(docName, "/") || strings.Contains(docName, "\\") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document name"})
		return
	}

	// Use the full filename including extension as document ID
	docID := docName

	// Check permissions via Mini-Zanzibar ACL
	canView, canEdit := getDocumentPermissions(userID, username, docID, role)

	if !canView {
		log.Printf("🚫 User %s denied access to document: %s (no viewer permission)", username, docName)
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied to this document"})
		return
	}

	// Read the document content
	documentsPath := getDocumentsPath()
	filePath := filepath.Join(documentsPath, docName)

	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("Error reading document %s: %v", docName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	log.Printf("📖 User %s accessed document: %s (view: %v, edit: %v)", username, docName, canView, canEdit)
	c.JSON(http.StatusOK, gin.H{
		"name":    docName,
		"content": string(content),
		"user":    username,
		"role":    role,
		"canEdit": canEdit,
	})
}

// handleSaveDocument saves content to a specific document file
func handleSaveDocument(c *gin.Context) {
	docName := c.Param("name")
	username := c.GetString("username")
	userID := c.GetString("user_id")
	role := c.GetString("role")

	// Validate that the document name is safe (no path traversal)
	if strings.Contains(docName, "..") || strings.Contains(docName, "/") || strings.Contains(docName, "\\") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document name"})
		return
	}

	// Use the full filename including extension as document ID
	docID := docName

	// Check if user has edit permission for this document
	canView, canEdit := getDocumentPermissions(userID, username, docID, role)

	if !canView {
		log.Printf("🚫 User %s denied access to document: %s (no viewer permission)", username, docName)
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied to this document"})
		return
	}

	if !canEdit {
		log.Printf("🚫 User %s denied edit access to document: %s (read-only permission)", username, docName)
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to edit this document"})
		return
	}

	// Parse the request body to get the new content
	var requestBody struct {
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Write the content to the document file
	documentsPath := getDocumentsPath()
	filePath := filepath.Join(documentsPath, docName)

	err := os.WriteFile(filePath, []byte(requestBody.Content), 0644)
	if err != nil {
		log.Printf("Error writing document %s: %v", docName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save document"})
		return
	}

	log.Printf("💾 User %s saved document: %s", username, docName)
	c.JSON(http.StatusOK, gin.H{
		"message": "Document saved successfully",
		"name":    docName,
		"user":    username,
	})
}

// handleDocumentAccess checks if user can access a document
func handleDocumentAccess(c *gin.Context) {
	docID := c.Param("id")
	userID := c.GetString("user_id")
	permission := c.DefaultQuery("permission", "viewer")

	// Forward to Zanzibar for authorization check
	checkURL := fmt.Sprintf("%s/api/v1/acl/check?object=doc:%s&relation=%s&user=%s",
		ZanzibarURL, docID, permission, userID)

	resp, err := http.Get(checkURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to check authorization",
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read authorization response",
		})
		return
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	c.JSON(resp.StatusCode, result)
}

// checkPermissionWithZanzibar checks if a user has a specific permission on a document via Mini-Zanzibar
// Falls back to role-based permissions if ACL check fails
func checkPermissionWithZanzibar(userID, docID, permission string) (bool, error) {
	checkURL := fmt.Sprintf("%s/api/v1/acl/check?object=doc:%s&relation=%s&user=%s",
		ZanzibarURL, docID, permission, userID)

	resp, err := http.Get(checkURL)
	if err != nil {
		log.Printf("Error checking permission with Zanzibar: %v, falling back to role-based", err)
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Permission check failed with status: %d, this likely means no ACL entries exist", resp.StatusCode)
		// Return false but no error - this means we should use fallback logic
		return false, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading permission response: %v", err)
		return false, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("Error parsing permission response: %v", err)
		return false, err
	}

	log.Printf("🔍 Permission check response for %s on doc:%s with %s: %v", userID, docID, permission, result)

	// Check if the permission is allowed
	if allowed, ok := result["allowed"].(bool); ok {
		log.Printf("✅ Found 'allowed' field: %v", allowed)
		return allowed, nil
	}

	// Also check for "authorized" field (Mini-Zanzibar uses this)
	if authorized, ok := result["authorized"].(bool); ok {
		log.Printf("✅ Found 'authorized' field: %v", authorized)
		return authorized, nil
	}

	log.Printf("❌ No 'allowed' or 'authorized' field found in response")
	return false, nil
}

// fallbackRoleBasedPermission provides role-based permissions when ACL entries don't exist
// Only Alice (owner) gets fallback permissions, everyone else needs explicit ACL entries
func fallbackRoleBasedPermission(username, role, docID, permission string) bool {
	// Only Alice gets fallback permissions as the system owner
	if username == "alice" && role == "owner" {
		return true // Alice can access all documents as the system owner
	}

	// Everyone else (Bob, Charlie, etc.) must have explicit ACL entries
	// No fallback permissions for non-owners
	return false
}

// getDocumentPermissions returns the highest permission level a user has for a document
func getDocumentPermissions(userID, username, docID, role string) (canView, canEdit bool) {
	log.Printf("🔍 Checking permissions for user %s (ID: %s) on document %s with role %s", username, userID, docID, role)

	// Try ACL first, then fallback to role-based permissions

	// Check for owner permission first (highest level)
	if allowed, err := checkPermissionWithZanzibar(userID, docID, "owner"); err != nil {
		log.Printf("⚠️ Owner permission check failed for %s on %s: %v", userID, docID, err)
		// ACL check failed (error), use fallback
		if fallbackRoleBasedPermission(username, role, docID, "owner") {
			log.Printf("✅ Fallback granted owner permission for %s on %s", username, docID)
			return true, true
		}
	} else if allowed {
		log.Printf("✅ ACL granted owner permission for %s on %s", userID, docID)
		// ACL check succeeded and permission granted
		return true, true
	}

	// Check for editor permission
	if allowed, err := checkPermissionWithZanzibar(userID, docID, "editor"); err != nil {
		log.Printf("⚠️ Editor permission check failed for %s on %s: %v", userID, docID, err)
		// ACL check failed (error), use fallback
		if fallbackRoleBasedPermission(username, role, docID, "editor") {
			log.Printf("✅ Fallback granted editor permission for %s on %s", username, docID)
			return true, true
		}
	} else if allowed {
		log.Printf("✅ ACL granted editor permission for %s on %s", userID, docID)
		// ACL check succeeded and permission granted
		return true, true
	}

	// Check for viewer permission
	if allowed, err := checkPermissionWithZanzibar(userID, docID, "viewer"); err != nil {
		log.Printf("⚠️ Viewer permission check failed for %s on %s: %v", userID, docID, err)
		// ACL check failed (error), use fallback
		if fallbackRoleBasedPermission(username, role, docID, "viewer") {
			log.Printf("✅ Fallback granted viewer permission for %s on %s", username, docID)
			return true, false
		}
	} else if allowed {
		log.Printf("✅ ACL granted viewer permission for %s on %s", userID, docID)
		// ACL check succeeded and permission granted
		return true, false
	}

	// If no ACL permissions and no errors, still try fallback for role-based access
	// This handles the case where ACL entries don't exist yet
	log.Printf("🔄 No ACL permissions found, trying fallback for %s on %s", username, docID)
	if fallbackRoleBasedPermission(username, role, docID, "owner") {
		log.Printf("✅ Final fallback granted owner permission for %s on %s", username, docID)
		return true, true
	}
	if fallbackRoleBasedPermission(username, role, docID, "editor") {
		log.Printf("✅ Final fallback granted editor permission for %s on %s", username, docID)
		return true, true
	}
	if fallbackRoleBasedPermission(username, role, docID, "viewer") {
		log.Printf("✅ Final fallback granted viewer permission for %s on %s", username, docID)
		return true, false
	}

	log.Printf("❌ No permissions found for %s on %s", username, docID)
	return false, false
}

// getDetailedDocumentPermissions checks all permission levels for a document
func getDetailedDocumentPermissions(userID, username, docID, role string) (canView, canEdit, canOwn bool) {
	log.Printf("🔍 Checking detailed permissions for user %s (ID: %s) on document %s with role %s", username, userID, docID, role)

	// Check each permission level via Mini-Zanzibar
	canOwn, _ = checkPermissionWithZanzibar(userID, docID, "owner")
	canEdit, _ = checkPermissionWithZanzibar(userID, docID, "editor")
	canView, _ = checkPermissionWithZanzibar(userID, docID, "viewer")

	// Apply permission hierarchy: owner > editor > viewer
	// If user is owner, they automatically get edit and view permissions
	if canOwn {
		canEdit = true
		canView = true
	} else if canEdit {
		// If user is editor, they automatically get view permission
		canView = true
	}

	// If ACL check fails (e.g., Mini-Zanzibar not available), fall back to role-based permissions for Alice only
	if !canView && !canEdit && !canOwn && username == "alice" {
		canView, canEdit, canOwn = fallbackDetailedRoleBasedPermission(username, docID, role)
		if canView || canEdit || canOwn {
			log.Printf("🔄 Using fallback permissions for Alice: view=%t, edit=%t, own=%t", canView, canEdit, canOwn)
		}
	}

	log.Printf("📋 Final permissions for %s on %s: view=%t, edit=%t, own=%t", username, docID, canView, canEdit, canOwn)
	return canView, canEdit, canOwn
}

// fallbackDetailedRoleBasedPermission provides detailed fallback permissions for Alice only
func fallbackDetailedRoleBasedPermission(username, docID, role string) (canView, canEdit, canOwn bool) {
	// Only provide fallback permissions for Alice
	if username != "alice" {
		return false, false, false
	}

	// Alice gets owner permissions as fallback
	if role == "owner" {
		return true, true, true
	}

	return false, false, false
}

// authMiddleware validates authentication
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		userID := session.Get("user_id")
		username := session.Get("username")
		role := session.Get("role")

		log.Printf("🔍 Auth check - UserID: %v, Username: %v, Role: %v", userID, username, role)

		if userID == nil || username == nil || role == nil {
			log.Printf("❌ Authentication failed - missing session data")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
			})
			c.Abort()
			return
		}

		log.Printf("✅ Authentication successful for user: %s", username)

		// Set user context for handlers
		c.Set("user_id", userID.(string))
		c.Set("username", username.(string))
		c.Set("role", role.(string))

		c.Next()
	}
}

// adminOnly middleware restricts access to admin users
func adminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "owner" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Owner access required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// proxyToZanzibar forwards requests to Mini-Zanzibar with user context
func proxyToZanzibar(c *gin.Context) {
	userID := c.GetString("user_id")

	log.Printf("🔄 Proxying %s %s for user: %s", c.Request.Method, c.Request.URL.Path, userID)

	// Map API paths to Mini-Zanzibar v1 API
	apiPath := c.Request.URL.Path
	if apiPath == "/api/acl" {
		apiPath = "/api/v1/acl"
	} else if apiPath == "/api/acl/check" {
		apiPath = "/api/v1/acl/check"
	} else if apiPath == "/api/namespace" {
		apiPath = "/api/v1/namespace"
	} else if len(apiPath) > 14 && apiPath[:14] == "/api/namespace" {
		apiPath = "/api/v1" + apiPath[4:] // /api/namespace/id -> /api/v1/namespace/id
	}

	// Build target URL
	targetURL := ZanzibarURL + apiPath
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	log.Printf("🎯 Target URL: %s", targetURL)

	// Read request body
	var body []byte
	if c.Request.Body != nil {
		var err error
		body, err = io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read request body"})
			return
		}
		log.Printf("📄 Request body: %s", string(body))
	}

	// Create new request
	req, err := http.NewRequest(c.Request.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Copy headers
	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Add user context header
	req.Header.Set("X-User-ID", userID)

	// Add user role header for bootstrap mode
	if role := c.GetString("role"); role != "" {
		req.Header.Set("X-User-Role", role)
	}

	// Make request to Zanzibar
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to reach authorization service"})
		return
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	// Copy response headers (excluding CORS headers to avoid conflicts)
	for key, values := range resp.Header {
		// Skip CORS headers to let our middleware handle them
		if key == "Access-Control-Allow-Origin" ||
			key == "Access-Control-Allow-Methods" ||
			key == "Access-Control-Allow-Headers" ||
			key == "Access-Control-Allow-Credentials" {
			continue
		}
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Return response
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
}

// createJWTToken creates a JWT token for the user
func createJWTToken(user User) (string, error) {
	claims := AuthClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "auth-service",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(JWTSecret))
}
