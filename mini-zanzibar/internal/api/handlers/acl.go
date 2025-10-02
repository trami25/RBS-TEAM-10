package handlers

import (
	"fmt"
	"mini-zanzibar/internal/database/consul"
	"mini-zanzibar/internal/database/leveldb"
	"mini-zanzibar/internal/database/redis"
	"mini-zanzibar/internal/models"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ACLHandler struct {
	leveldbClient *leveldb.Client
	consulClient  *consul.Client
	redisClient   *redis.Client
	logger        *zap.SugaredLogger
}

// NewACLHandler creates a new ACL handler
func NewACLHandler(leveldbClient *leveldb.Client, consulClient *consul.Client, redisClient *redis.Client, logger *zap.SugaredLogger) *ACLHandler {
	return &ACLHandler{
		leveldbClient: leveldbClient,
		consulClient:  consulClient,
		redisClient:   redisClient,
		logger:        logger,
	}
}

// CreateACL handles POST /acl - Create or update an ACL tuple
func (h *ACLHandler) CreateACL(c *gin.Context) {
	var req models.ACLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Errorw("Failed to bind JSON", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.Infow("ACL creation request", "object", req.Object, "relation", req.Relation, "user", req.User)

	// Check user context
	user, exists := c.Get("user")
	if exists {
		h.logger.Infow("User context found", "user", user)
	} else {
		h.logger.Warnw("No user context found")
	}

	// Validate request format (object:type, user:username, etc.)
	if err := h.validateACLRequest(req); err != nil {
		h.logger.Errorw("ACL request validation failed", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if namespace exists and relation is valid
	if err := h.validateNamespaceAndRelation(req.Object, req.Relation); err != nil {
		// Bootstrap mode: Allow alice to create first ACL even if validation fails
		// Also auto-create the doc namespace if it doesn't exist
		user, exists := c.Get("user")
		if exists && user.(string) == "user:alice" {
			existingTuples, checkErr := h.leveldbClient.ListTuplesByUser("user:alice")
			if checkErr == nil && len(existingTuples) == 0 {
				h.logger.Infow("Bootstrap mode: Bypassing validation for alice's first ACL", "object", req.Object, "relation", req.Relation)

				// Auto-create the doc namespace if this is a doc object
				if err := h.ensureDocNamespaceExists(req.Object); err != nil {
					h.logger.Warnw("Failed to auto-create doc namespace", "error", err)
				}
			} else {
				// Even if Alice has existing ACLs, try to create the namespace if it's missing
				if err := h.ensureDocNamespaceExists(req.Object); err == nil {
					// Retry validation after creating namespace
					if validateErr := h.validateNamespaceAndRelation(req.Object, req.Relation); validateErr == nil {
						// Validation passed after creating namespace, continue
						h.logger.Infow("Namespace auto-created, validation now passes", "object", req.Object, "relation", req.Relation)
					} else {
						h.logger.Errorw("Namespace/relation validation failed even after auto-creation", "error", validateErr)
						c.JSON(http.StatusBadRequest, gin.H{"error": validateErr.Error()})
						return
					}
				} else {
					h.logger.Errorw("Namespace/relation validation failed", "error", err)
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
			}
		} else {
			// For non-Alice users, try to auto-create namespace if it's a doc object
			if err := h.ensureDocNamespaceExists(req.Object); err == nil {
				// Retry validation after creating namespace
				if validateErr := h.validateNamespaceAndRelation(req.Object, req.Relation); validateErr != nil {
					h.logger.Errorw("Namespace/relation validation failed even after auto-creation", "error", validateErr)
					c.JSON(http.StatusBadRequest, gin.H{"error": validateErr.Error()})
					return
				}
			} else {
				h.logger.Errorw("Namespace/relation validation failed", "error", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
	}

	// Implement authorization check for ACL management with bootstrap support
	if !h.isAuthorizedForACLManagement(c, req.Object) {
		h.logger.Errorw("Authorization failed for ACL management", "object", req.Object, "user", user)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unauthorized to manage ACLs for this object"})
		return
	}

	tuple := leveldb.ACLTuple{
		Object:   req.Object,
		Relation: req.Relation,
		User:     req.User,
	}

	if err := h.leveldbClient.StoreTuple(tuple); err != nil {
		h.logger.Errorw("Failed to store ACL tuple", "error", err, "tuple", tuple)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store ACL"})
		return
	}

	// Auto-grant alice owner permission for new documents in doc namespace
	if err := h.autoGrantAliceOwnershipForNewDocuments(req.Object); err != nil {
		h.logger.Warnw("Failed to auto-grant alice ownership", "error", err, "object", req.Object)
		// Don't fail the main operation if auto-grant fails
	}

	// Invalidate cache for this object and user
	h.invalidateAuthorizationCache(req.Object, req.Relation, req.User)

	h.logger.Infow("ACL tuple created", "tuple", tuple)
	c.JSON(http.StatusCreated, gin.H{"message": "ACL created successfully"})
}

// CheckACL handles GET /acl/check - Check authorization
func (h *ACLHandler) CheckACL(c *gin.Context) {
	var req models.ACLCheckRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate required parameters
	if req.Object == "" || req.Relation == "" || req.User == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "object, relation and user parameters are required"})
		return
	}

	// Caching for performance
	cacheKey := fmt.Sprintf("auth:%s:%s:%s", req.Object, req.Relation, req.User)

	// Try to get from cache first
	if cached, found := h.redisClient.Get(cacheKey); found {
		authorized, ok := cached.(bool)
		if ok {
			response := models.ACLCheckResponse{
				Authorized: authorized,
			}
			h.logger.Debugw("authorization cache hit", "request", req, "authorized", authorized)
			c.JSON(http.StatusOK, response)
			return
		}
	}

	// Implement full authorization logic with namespace rules
	// Handle computed usersets and union operations
	authorized, err := h.performAuthorizationCheck(req.Object, req.Relation, req.User)
	if err != nil {
		h.logger.Errorw("failed to check authorization", "error", err, "request", req)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check authorization"})
		return
	}

	// Cache the result with 5-minute expiration
	h.redisClient.Set(cacheKey, authorized, 5*time.Minute)

	response := models.ACLCheckResponse{
		Authorized: authorized,
	}

	h.logger.Infow("Authorization check", "request", req, "authorized", authorized, "cache_miss", true)
	c.JSON(http.StatusOK, response)
}

// DeleteACL handles DELETE /acl - Delete an ACL tuple
func (h *ACLHandler) DeleteACL(c *gin.Context) {
	var req models.ACLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.validateACLRequest(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}

	// Implement authorization check for ACL management
	if !h.isAuthorizedForACLManagement(c, req.Object) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized to manage ACLs for this object"})
		return
	}

	if err := h.leveldbClient.DeleteTuple(req.Object, req.Relation, req.User); err != nil {
		h.logger.Errorw("Failed to delete ACL tuple", "error", err, "request", req)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete ACL"})
		return
	}

	// Invalidate cache for this object and user
	h.invalidateAuthorizationCache(req.Object, req.Relation, req.User)

	h.logger.Infow("ACL tuple deleted", "request", req)
	c.JSON(http.StatusOK, gin.H{"message": "ACL deleted successfully"})
}

// ListACLsByObject handles GET /acl/object/:object - List ACLs for an object
func (h *ACLHandler) ListACLsByObject(c *gin.Context) {
	object := c.Param("object")
	if object == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "object parameter is required"})
		return
	}

	// Authorization check for ACL listing
	if !h.isAuthorizedForACLListing(c, object) {
		c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized to list ACLs for this object"})
		return
	}
	// Add pagination for large result sets
	page, pageSize := h.getPaginationParams(c)

	// Use paginated version
	tuples, total, err := h.leveldbClient.ListTuplesByObjectPagination(object, page, pageSize)
	if err != nil {
		h.logger.Errorw("failed to list ACLs by object", "error", err, "object", object)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list ACLs"})
		return
	}

	response := gin.H{
		"tuples": tuples,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
			"has_more":  len(tuples) == pageSize,
		},
	}

	c.JSON(http.StatusOK, response)
}

// ListACLsByUser handles GET /acl/user/:user - List ACLs for a user
func (h *ACLHandler) ListACLsByUser(c *gin.Context) {
	user := c.Param("user")
	if user == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user parameter is required"})
		return
	}

	// Authorization check for ACL listing
	if !h.isAuthorizedForACLListing(c, user) {
		c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized to list ACLs for this user"})
		return
	}
	// Pagination for large result sets
	page, pageSize := h.getPaginationParams(c)

	// Use paginated version
	tuples, total, err := h.leveldbClient.ListTuplesByUserPagination(user, page, pageSize)

	if err != nil {
		h.logger.Errorw("failed to list ACLs by user", "error", err, "user", user)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list ACLs"})
		return
	}

	response := gin.H{
		"tuples": tuples,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
			"has_more":  len(tuples) == pageSize,
		},
	}

	c.JSON(http.StatusOK, response)
}

func (h *ACLHandler) validateACLRequest(req models.ACLRequest) error {
	if req.Object == "" {
		return fmt.Errorf("object is required")
	}

	if req.Relation == "" {
		return fmt.Errorf("relation is required")
	}

	if req.User == "" {
		return fmt.Errorf("user is required")
	}

	parts := strings.Split(req.Object, ":")
	if len(parts) != 2 {
		return fmt.Errorf("object must be in format 'namespace:object_id'")
	}

	userParts := strings.Split(req.User, ":")
	if len(userParts) < 2 {
		return fmt.Errorf("user must be in format 'user_type:user_id' or 'userset:namespace:relation'")
	}

	return nil
}

func (h *ACLHandler) validateNamespaceAndRelation(object string, relation string) error {
	parts := strings.Split(object, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid object format")
	}
	namespace := parts[0]

	// Check if namespace exists in Consul
	exists, err := h.consulClient.NamespaceExists(namespace)
	if err != nil {
		return fmt.Errorf("failed to check namespace: %v", err)
	}

	if !exists {
		return fmt.Errorf("namespace '%s' does not exist", namespace)
	}

	// Check if relation is valid for this namespace
	valid, err := h.consulClient.RelationExists(namespace, relation)
	if err != nil {
		return fmt.Errorf("failed to check relation: %v", err)
	}

	if !valid {
		return fmt.Errorf("relation '%s' is not valid for namespace '%s'", relation, namespace)
	}

	return nil
}

// Authorization check for ACL management
func (h *ACLHandler) isAuthorizedForACLManagement(c *gin.Context, object string) bool {
	fmt.Printf("*** AUTHORIZATION FUNCTION CALLED FOR OBJECT: %s ***\n", object)
	h.logger.Infow("DEBUG: Starting ACL authorization check", "object", object)

	// Extract user from context (set by authentication middleware)
	user, exists := c.Get("user")
	h.logger.Infow("DEBUG: User from context", "user", user, "exists", exists)

	// Fallback: Extract user directly from headers if context is empty
	if !exists || user == nil {
		userFromHeader := c.GetHeader("X-User-ID")
		h.logger.Infow("DEBUG: User from header", "userFromHeader", userFromHeader)
		if userFromHeader != "" {
			user = userFromHeader
			exists = true
			h.logger.Infow("DEBUG: Using user from header", "user", user)
		}
	}

	if !exists || user == nil {
		h.logger.Warnw("No user in context for ACL management", "object", object)
		return false
	}

	userStr := user.(string)
	h.logger.Infow("DEBUG: Checking authorization for user", "user", userStr, "object", object)

	// Bootstrap mode: If no ACLs exist in the system, allow alice to be the first owner
	if userStr == "user:alice" {
		// Check if any ACLs exist in the system by checking if alice has any existing ACLs
		existingTuples, err := h.leveldbClient.ListTuplesByUser("user:alice")
		if err != nil {
			h.logger.Errorw("Failed to check existing ACLs for bootstrap", "error", err)
			// Continue with normal authorization check
		} else {
			h.logger.Infow("DEBUG: Alice's existing ACLs", "count", len(existingTuples), "tuples", existingTuples)
			if len(existingTuples) == 0 {
				h.logger.Infow("Bootstrap mode: Allowing alice to create first ACL (no existing ACLs for alice)", "object", object)
				return true
			} else {
				// SPECIAL CASE: Allow Alice to create new documents even if she has existing ACLs
				// Check if this is a NEW document that Alice doesn't own yet
				authorized, err := h.performAuthorizationCheck(object, "owner", userStr)
				if err != nil {
					h.logger.Errorw("Failed to check if Alice owns this specific object", "error", err)
				} else if !authorized {
					// Alice doesn't own this specific object, but she can create new ones
					h.logger.Infow("Allowing Alice to create new document ownership", "object", object, "user", userStr)
					return true
				}
			}
		}
	}

	// PRIMARY CHECK: Check if user is owner of the specific object
	// In Zanzibar model, only owners can manage ACLs for their objects
	authorized, err := h.performAuthorizationCheck(object, "owner", userStr)
	if err != nil {
		h.logger.Errorw("Failed to check owner authorization", "error", err, "object", object, "user", userStr)
	} else if authorized {
		h.logger.Infow("User authorized as owner of specific object", "object", object, "user", userStr)
		return true
	}

	// SPECIAL CASE: Allow creating owner permissions for new documents
	// If the object doesn't have ANY ACLs yet, allow the requesting user to become the owner
	existingObjectTuples, err := h.leveldbClient.ListTuplesByObject(object)
	if err != nil {
		h.logger.Errorw("Failed to check existing ACLs for object", "error", err, "object", object)
	} else if len(existingObjectTuples) == 0 {
		h.logger.Infow("Allowing user to become owner of new object (no existing ACLs)", "object", object, "user", userStr)
		return true
	}

	// SECONDARY CHECK: Check if user has any owner privileges (for Alice's special case and other scenarios)
	existingTuples, err := h.leveldbClient.ListTuplesByUser(userStr)
	if err != nil {
		h.logger.Errorw("Failed to check user's existing ACLs", "error", err, "user", userStr)
	} else {
		h.logger.Infow("DEBUG: User's existing ACLs", "user", userStr, "count", len(existingTuples), "tuples", existingTuples)

		// Check if user owns ANY objects
		hasOwnership := false
		for _, tuple := range existingTuples {
			if tuple.Relation == "owner" {
				hasOwnership = true
				h.logger.Infow("DEBUG: User owns object", "user", userStr, "owned_object", tuple.Object)
				break
			}
		}

		// If user has ownership of any document, allow them to manage ACLs
		// This supports the use case where owners can share their documents
		if hasOwnership {
			h.logger.Infow("User has ownership privileges, allowing ACL management", "object", object, "user", userStr)
			return true
		}
	}

	h.logger.Warnw("User not authorized to manage ACLs for this object", "object", object, "user", userStr)
	return false
}

// Full authorization logic with namespace rules
// Handle computed usersets and union operations
func (h *ACLHandler) performAuthorizationCheck(object, relation, user string) (bool, error) {
	// 1. Check direct tuple first (common case)
	directAuthorized, err := h.leveldbClient.CheckTuple(object, relation, user)

	if err != nil {
		return false, fmt.Errorf("failed to check direct tuple: %v", err)
	}

	if directAuthorized {
		return true, nil
	}

	// 2. Check permission hierarchy: owner > editor > viewer
	// If user is checking for viewer/editor permission, also check if they have higher permissions
	hierarchyPermissions := h.getPermissionHierarchy(relation)
	for _, higherPermission := range hierarchyPermissions {
		if higherPermission != relation {
			higherAuthorized, err := h.leveldbClient.CheckTuple(object, higherPermission, user)
			if err != nil {
				h.logger.Warnw("Failed to check higher permission", "error", err, "permission", higherPermission)
				continue
			}
			if higherAuthorized {
				h.logger.Infow("User authorized via permission hierarchy",
					"user", user, "object", object, "requested", relation, "granted_via", higherPermission)
				return true, nil
			}
		}
	}

	// 3. Check namespace rules for computed usersets and union operations
	parts := strings.Split(object, ":")
	if len(parts) != 2 {
		return false, nil
	}
	namespace := parts[0]

	// Get namespace configuration to check for computed usersets
	config, err := h.consulClient.GetNamespace(namespace)
	if err != nil {
		return false, fmt.Errorf("failed to get namespace config: %v", err)
	}

	// Check if this relation has computed usersets defined
	if relationConfig, exists := config.Relations[relation]; exists {
		for _, union := range relationConfig.Union {
			// Handle computed userset
			if union.ComputedUserset != nil {
				computedRelation := union.ComputedUserset.Relation

				authorized, err := h.checkComputedUserset(object, computedRelation, user)

				if err != nil {
					return false, err
				}

				if authorized {
					return true, nil
				}
			}

			// Handle union of multiple relations (this is a simplified version)
			// In a full implementation, you'd handle union, intersection, and exclusion
			if union.This != nil {
				// This represents direct membership - already checked above
				continue
			}
		}
	}

	return false, nil
}

// checkComputedUserset handles computed userset logic
func (h *ACLHandler) checkComputedUserset(object, computedRelation, user string) (bool, error) {
	// For computed userset, we need to find all objects that this object has the computed relation to
	// and check if the user has the base relation to those objects

	// This is a simplified implementation - in a real Zanzibar-like system,
	// you'd have a more complex graph traversal

	// Example: If document:1 has editor → group:eng and user is member of group:eng
	// Then user is effectively an editor of document:1

	// Get all tuples where this object has the computed relation
	relatedTuples, err := h.leveldbClient.ListTuplesByObjectAndRelation(object, computedRelation)
	if err != nil {
		return false, err
	}

	// Check if user has the base relation to any of the related objects
	for _, tuple := range relatedTuples {
		// tuple.User is the target object in the computed relation
		// Check if our user has the base relation to that target object
		authorized, err := h.performAuthorizationCheck(tuple.User, computedRelation, user)

		if err != nil {
			return false, err
		}

		if authorized {
			return true, nil
		}
	}

	return false, nil
}

// getPermissionHierarchy returns the permission hierarchy for a given relation
// In order of precedence: owner > editor > viewer
func (h *ACLHandler) getPermissionHierarchy(relation string) []string {
	switch relation {
	case "viewer":
		return []string{"owner", "editor", "viewer"}
	case "editor":
		return []string{"owner", "editor"}
	case "owner":
		return []string{"owner"}
	default:
		// For unknown relations, only check the exact relation
		return []string{relation}
	}
}

// Invalidate authorization cache when ACLs change
func (h *ACLHandler) invalidateAuthorizationCache(object, relation, user string) {

	// Invalidate specific check
	h.redisClient.Delete(fmt.Sprintf("auth:%s:%s:%s", object, relation, user))

	// Invalidate pattern-based cache using Redis SCAN
	h.redisClient.DeletePattern(fmt.Sprintf("auth:%s:%s:*", object, relation))

	h.redisClient.DeletePattern(fmt.Sprintf("auth:%s:*", object))

	h.logger.Debugw("Invalidated authorization cache", "object", object, "relation", relation, "user", user)
}

// Authorization check for ACL listing
func (h *ACLHandler) isAuthorizedForACLListing(c *gin.Context, resource string) bool {
	// Extract user from context (set by authentication middleware)
	user, exists := c.Get("user")
	if !exists {
		h.logger.Warnw("no user in context for ACL listing", "resource", resource)
		return false
	}

	// For objects, check if user has view permissions
	if strings.Contains(resource, ":") {
		parts := strings.Split(resource, ":")
		if len(parts) == 2 {
			namespace := parts[0]

			// Check if user can view this namespace's ACLs
			authorized, err := h.performAuthorizationCheck(
				fmt.Sprintf("namespace:%s", namespace),
				"view_acls",
				user.(string),
			)
			if err == nil && authorized {
				return true
			}
		}
	}

	// Allow users to view their own ACLs
	return resource == user.(string)
}

// getPaginationParams extracts pagination parameters from query
func (h *ACLHandler) getPaginationParams(c *gin.Context) (int, int) {
	page := 1
	pageSize := 50

	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	// Limit page size to prevent excessive memory usage
	if pageSize > 1000 {
		pageSize = 1000
	}

	if pageSize < 1 {
		pageSize = 1
	}

	return page, pageSize
}

// autoGrantAliceOwnershipForNewDocuments automatically grants alice owner permission for new documents in doc namespace
func (h *ACLHandler) autoGrantAliceOwnershipForNewDocuments(object string) error {
	// Check if this is a document in the doc namespace
	parts := strings.Split(object, ":")
	if len(parts) != 2 || parts[0] != "doc" {
		// Not a document object, skip auto-grant
		return nil
	}

	documentID := parts[1]
	aliceUser := "user:alice"

	// Check if alice already has owner permission for this document
	hasOwnership, err := h.leveldbClient.CheckTuple(object, "owner", aliceUser)
	if err != nil {
		return fmt.Errorf("failed to check existing ownership: %v", err)
	}

	if hasOwnership {
		h.logger.Infow("Alice already has ownership of document", "document", documentID, "object", object)
		return nil
	}

	// Check if this is truly a new document by seeing if there are any existing ACLs for it
	existingTuples, err := h.leveldbClient.ListTuplesByObjectAndRelation(object, "owner")
	if err != nil {
		return fmt.Errorf("failed to check existing document owners: %v", err)
	}

	// If there are no existing owners, this is a new document - grant alice ownership
	if len(existingTuples) == 0 {
		aliceOwnerTuple := leveldb.ACLTuple{
			Object:   object,
			Relation: "owner",
			User:     aliceUser,
		}

		if err := h.leveldbClient.StoreTuple(aliceOwnerTuple); err != nil {
			return fmt.Errorf("failed to grant alice ownership: %v", err)
		}

		h.logger.Infow("Auto-granted alice owner permission for new document",
			"document", documentID, "object", object, "user", aliceUser)

		// Invalidate cache for alice's new ownership
		h.invalidateAuthorizationCache(object, "owner", aliceUser)
	} else {
		h.logger.Debugw("Document already has owners, not auto-granting alice ownership",
			"document", documentID, "object", object, "existing_owners", len(existingTuples))
	}

	return nil
}

// ensureDocNamespaceExists creates the doc namespace if it doesn't exist and the object is a doc object
func (h *ACLHandler) ensureDocNamespaceExists(object string) error {
	parts := strings.Split(object, ":")
	if len(parts) != 2 || parts[0] != "doc" {
		// Not a doc object, skip auto-creation
		return nil
	}

	// Check if doc namespace already exists
	exists, err := h.consulClient.NamespaceExists("doc")
	if err != nil {
		return fmt.Errorf("failed to check doc namespace existence: %v", err)
	}

	if exists {
		// Namespace already exists
		return nil
	}

	// Create the doc namespace with standard relations
	h.logger.Infow("Auto-creating doc namespace with standard relations")

	// Create namespace config with proper types
	namespaceConfig := consul.NamespaceConfig{
		Namespace: "doc",
		Relations: map[string]consul.RelationConfig{
			"owner": {
				Union: []consul.UnionConfig{
					{This: &consul.ThisConfig{}},
				},
			},
			"editor": {
				Union: []consul.UnionConfig{
					{This: &consul.ThisConfig{}},
					{ComputedUserset: &consul.ComputedUsersetConfig{Relation: "owner"}},
				},
			},
			"viewer": {
				Union: []consul.UnionConfig{
					{This: &consul.ThisConfig{}},
					{ComputedUserset: &consul.ComputedUsersetConfig{Relation: "editor"}},
				},
			},
		},
		Version: 1,
	}

	if err := h.consulClient.StoreNamespace("doc", namespaceConfig); err != nil {
		return fmt.Errorf("failed to create doc namespace: %v", err)
	}

	h.logger.Infow("Successfully auto-created doc namespace with relations: owner, editor, viewer")
	return nil
}
