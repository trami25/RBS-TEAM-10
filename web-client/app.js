// Configuration
const AUTH_BASE_URL = 'http://localhost:8081';
const API_BASE_URL = 'http://localhost:8081/api';
let currentUser = null;
let isAuthenticated = false;
let autoRefreshInterval = null;
let lastDocumentCount = 0;

// Initialize the application
document.addEventListener('DOMContentLoaded', function() {
    initializeApp();
});

function initializeApp() {
    // Check authentication status first
    checkAuthStatus();
    
    // Check if auth service is running
    checkAuthServiceStatus();
    
    // Initialize UI
    initializeUI();
    
    logActivity('Application initialized. Please login to continue.');
    
    // Test connection to auth service
    testAuthServiceConnection();
}

// Test connection to auth service
async function testAuthServiceConnection() {
    try {
        console.log('Testing connection to auth service...');
        const response = await fetch(`${AUTH_BASE_URL}/health`);
        if (response.ok) {
            const data = await response.json();
            console.log('Auth service connection test successful:', data);
            logActivity('✅ Auth service connection test successful', 'success');
        } else {
            throw new Error(`Auth service returned status: ${response.status}`);
        }
    } catch (error) {
        console.error('Auth service connection test failed:', error);
        logActivity(`❌ Auth service connection test failed: ${error.message}`, 'error');
    }
}

// Authentication functions
async function checkAuthStatus() {
    try {
        const response = await makeAuthCall('/auth/me');
        if (response.user_id) {
            currentUser = { username: response.user_id, role: response.role };
            isAuthenticated = true;
            hideLoginForm();
            updateUserDisplay();
            startAutoRefresh(); // Start automatic refresh for real-time updates
        }
    } catch (error) {
        // Not authenticated, show login form
        isAuthenticated = false;
        currentUser = null;
        showLoginForm();
    }
}

async function login(username, password) {
    try {
        const response = await fetch(`${AUTH_BASE_URL}/auth/login`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            credentials: 'include',
            body: JSON.stringify({ username, password })
        });

        if (response.ok) {
            const data = await response.json();
            
            // Get additional user info if role not provided in login response
            let userRole = data.role;
            if (!userRole) {
                try {
                    const userInfo = await makeAuthCall('/auth/me');
                    userRole = userInfo.role;
                } catch (error) {
                    console.warn('Could not fetch user role:', error);
                    userRole = 'owner'; // Default for Alice
                }
            }
            
            currentUser = { username: data.user_id, role: userRole };
            isAuthenticated = true;
            hideLoginForm();
            updateUserDisplay();
            startAutoRefresh(); // Start automatic refresh for real-time updates
            logActivity(`✅ Login successful: ${data.user_id} (${userRole})`, 'success');
            
            // Load documents after successful login
            showSection('documents');
            loadDocuments();
        } else {
            const errorData = await response.json();
            throw new Error(errorData.error || 'Login failed');
        }
    } catch (error) {
        showLoginError(error.message);
        logActivity(`❌ Login failed: ${error.message}`, 'error');
    }
}

async function logout() {
    try {
        stopAutoRefresh(); // Stop automatic refresh
        await makeAuthCall('/auth/logout', 'POST');
        currentUser = null;
        isAuthenticated = false;
        showLoginForm();
        logActivity('✅ Logout successful', 'success');
    } catch (error) {
        logActivity(`❌ Logout error: ${error.message}`, 'error');
    }
}

async function makeAuthCall(endpoint, method = 'GET', data = null) {
    try {
        const config = {
            method: method,
            headers: {
                'Content-Type': 'application/json',
            },
            credentials: 'include'
        };
        
        if (data) {
            config.body = JSON.stringify(data);
        }
        
        const response = await fetch(`${AUTH_BASE_URL}${endpoint}`, config);
        
        if (response.status === 401) {
            isAuthenticated = false;
            currentUser = null;
            showLoginForm();
            throw new Error('Session expired. Please login again.');
        }
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        return await response.json();
    } catch (error) {
        console.error('Auth call failed:', error);
        logActivity(`Auth Error: ${error.message}`, 'error');
        throw error;
    }
}

async function makeApiCall(endpoint, method = 'GET', data = null) {
    if (!isAuthenticated) {
        throw new Error('Authentication required');
    }
    
    try {
        const config = {
            method: method,
            headers: {
                'Content-Type': 'application/json',
            },
            credentials: 'include'
        };
        
        if (data) {
            config.body = JSON.stringify(data);
        }
        
        const response = await fetch(`${API_BASE_URL}${endpoint}`, config);
        
        if (response.status === 401) {
            isAuthenticated = false;
            currentUser = null;
            showLoginForm();
            throw new Error('Session expired. Please login again.');
        }
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        return await response.json();
    } catch (error) {
        console.error('API call failed:', error);
        logActivity(`API Error: ${error.message}`, 'error');
        throw error;
    }
}

// Service status checks
async function checkAuthServiceStatus() {
    try {
        const response = await fetch(`${AUTH_BASE_URL}/health`);
        if (response.ok) {
            logActivity('✅ Connected to Auth Service successfully', 'success');
            updateConnectionStatus(true);
        } else {
            throw new Error('Auth service unhealthy');
        }
    } catch (error) {
        logActivity('❌ Failed to connect to Auth Service. Make sure it\'s running on port 8081.', 'error');
        updateConnectionStatus(false);
    }
}

// Auto-refresh functionality for real-time ACL updates
function startAutoRefresh() {
    if (autoRefreshInterval) {
        clearInterval(autoRefreshInterval);
    }
    
    // Check for updates every 5 seconds
    autoRefreshInterval = setInterval(async () => {
        if (isAuthenticated && currentUser) {
            await checkForUpdates();
        }
    }, 5000);
    
    // Update status indicator
    const statusElement = document.getElementById('auto-refresh-status');
    if (statusElement) {
        statusElement.textContent = '🔄 Auto-refresh active';
        statusElement.style.color = '#28a745';
    }
    
    logActivity('🔄 Started automatic refresh for real-time updates', 'info');
}

function stopAutoRefresh() {
    if (autoRefreshInterval) {
        clearInterval(autoRefreshInterval);
        autoRefreshInterval = null;
        
        // Update status indicator
        const statusElement = document.getElementById('auto-refresh-status');
        if (statusElement) {
            statusElement.textContent = '⏹️ Auto-refresh stopped';
            statusElement.style.color = '#6c757d';
        }
        
        logActivity('⏹️ Stopped automatic refresh', 'info');
    }
}

async function checkForUpdates() {
    try {
        // Check if document count has changed (new documents shared with user)
        const response = await fetch(`${AUTH_BASE_URL}/documents`, {
            method: 'GET',
            credentials: 'include'
        });
        
        if (response.ok) {
            const data = await response.json();
            const currentDocumentCount = data.documents ? data.documents.length : 0;
            
            // If document count changed, refresh the UI
            if (lastDocumentCount !== 0 && currentDocumentCount !== lastDocumentCount) {
                const change = currentDocumentCount > lastDocumentCount ? 'gained' : 'lost';
                logActivity(`📄 Document access updated! You ${change} access to ${Math.abs(currentDocumentCount - lastDocumentCount)} document(s)`, 'success');
                
                // Show visual notification
                showUpdateNotification(`Document access updated! You ${change} access to ${Math.abs(currentDocumentCount - lastDocumentCount)} document(s)`);
                
                // Refresh documents list if on documents tab
                if (document.getElementById('documents').style.display !== 'none') {
                    await loadDocuments();
                }
                
                // Refresh permissions if on test authorization tab
                if (document.getElementById('test-authorization').style.display !== 'none') {
                    await refreshMyPermissions();
                    await loadDocumentsForTesting();
                }
            }
            
            lastDocumentCount = currentDocumentCount;
        }
    } catch (error) {
        // Silently handle errors for background refresh
        console.log('Background refresh error (normal):', error.message);
    }
}

// UI Management
function initializeUI() {
    setupLoginForm();
    setupNavigation();
    setupLogoutButton();
    
    if (!isAuthenticated) {
        showLoginForm();
    }
}

function setupLoginForm() {
    const loginForm = document.getElementById('login-form');
    if (loginForm) {
        loginForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;
            await login(username, password);
        });
    }
}

function setupNavigation() {
    document.getElementById('nav-documents').addEventListener('click', () => {
        if (isAuthenticated) {
            showSection('documents');
            loadDocuments();
        }
    });
    
    document.getElementById('nav-access').addEventListener('click', () => {
        if (isAuthenticated) {
            showSection('access-control');
        }
    });
    
    document.getElementById('nav-test').addEventListener('click', () => {
        if (isAuthenticated) {
            showSection('test-authorization');
        }
    });
}

function setupLogoutButton() {
    const logoutBtn = document.getElementById('logout-btn');
    if (logoutBtn) {
        logoutBtn.addEventListener('click', logout);
    }
}

function showLoginForm() {
    document.getElementById('login-section').style.display = 'block';
    document.getElementById('main-content').style.display = 'none';
    document.getElementById('user-info').style.display = 'none';
}

function hideLoginForm() {
    document.getElementById('login-section').style.display = 'none';
    document.getElementById('main-content').style.display = 'block';
    document.getElementById('user-info').style.display = 'block';
}

function showLoginError(message) {
    const errorDiv = document.getElementById('login-error');
    if (errorDiv) {
        errorDiv.textContent = message;
        errorDiv.style.display = 'block';
    }
}

function updateUserDisplay() {
    if (currentUser) {
        const userInfo = document.getElementById('user-info');
        const userDisplay = document.getElementById('current-user');
        const roleDisplay = document.getElementById('current-role');
        
        if (userDisplay) userDisplay.textContent = currentUser.username;
        if (roleDisplay) roleDisplay.textContent = currentUser.role;
        if (userInfo) userInfo.style.display = 'block';
    }
}

// Document Management with File System Integration
async function loadDocuments() {
    try {
        logActivity('🔄 Loading documents...', 'info');
        
        // Get documents from the auth service which checks Mini-Zanzibar permissions
        const response = await fetch(`${AUTH_BASE_URL}/documents`, {
            method: 'GET',
            credentials: 'include'
        });
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        const data = await response.json();
        console.log('DEBUG: Documents received from server:', data);
        
        const documentsContainer = document.getElementById('documents-list');
        
        if (data.documents && data.documents.length > 0) {
            // Store document count for auto-refresh detection
            lastDocumentCount = data.documents.length;
            
            console.log('DEBUG: Processing documents:', data.documents);
            
            // The backend now returns detailed permission information for each document
            documentsContainer.innerHTML = data.documents.map(doc => {
                console.log('DEBUG: Processing document:', doc, typeof doc);
                
                // Ensure doc is an object with the expected properties
                if (typeof doc !== 'object' || !doc.name) {
                    console.error('ERROR: Invalid document format:', doc);
                    return ''; // Skip invalid documents
                }
                
                // doc is now an object with name and permission flags
                const docName = doc.name;
                const canView = doc.canView;
                const canEdit = doc.canEdit;
                const canOwn = doc.canOwn;
                
                console.log(`DEBUG: Document ${docName} permissions: view=${canView}, edit=${canEdit}, own=${canOwn}`);
                
                // Skip documents where user has no permissions at all
                if (!canView && !canEdit && !canOwn) {
                    console.log(`DEBUG: Skipping ${docName} - no permissions`);
                    return ''; // Don't display this document
                }
                
                // Determine the highest permission level for the badge
                const permissionLevel = canOwn ? 'owner' : (canEdit ? 'editor' : 'viewer');
                const permissionColor = canOwn ? '#28a745' : (canEdit ? '#ffc107' : '#6c757d');
                
                return `
                    <div class="document-item">
                        <div class="document-header">
                            <h3>${docName}</h3>
                            <span class="permission-badge" style="background-color: ${permissionColor}; color: white; padding: 2px 8px; border-radius: 4px; font-size: 12px;">
                                ${permissionLevel}
                            </span>
                        </div>
                        <div class="document-actions">
                            ${canView ? `
                                <button onclick="openDocument('${docName}')" class="btn-primary">
                                    📖 View
                                </button>
                            ` : ''}
                            ${canEdit ? `
                                <button onclick="editDocument('${docName}')" class="btn-secondary">
                                    ✏️ Edit
                                </button>
                            ` : ''}
                            ${canOwn ? `
                                <button onclick="shareDocument('${docName}')" class="btn-tertiary">
                                    📤 Share
                                </button>
                            ` : ''}
                        </div>
                    </div>
                `;
            }).filter(html => html !== '').join(''); // Filter out empty strings from documents without permissions
        } else {
            documentsContainer.innerHTML = '<p>No documents available for your role.</p>';
        }
        
        logActivity(`📄 Loaded ${data.documents ? data.documents.length : 0} documents for user: ${data.user} (${data.role})`, 'success');
    } catch (error) {
        logActivity(`Failed to load documents: ${error.message}`, 'error');
        const documentsContainer = document.getElementById('documents-list');
        documentsContainer.innerHTML = '<p class="error">Error loading documents. Please try again.</p>';
    }
}

async function openDocument(docName) {
    try {
        // Debug: Check what we received
        console.log('DEBUG: openDocument called with:', docName, typeof docName);
        
        // Ensure docName is a string
        if (typeof docName !== 'string') {
            console.error('ERROR: Document name is not a string:', docName);
            logActivity(`❌ Error: Invalid document name format`, 'error');
            return;
        }
        
        logActivity(`📖 Opening document: ${docName}...`, 'info');
        
        const response = await fetch(`${AUTH_BASE_URL}/documents/${docName}`, {
            method: 'GET',
            credentials: 'include'
        });
        
        if (response.status === 403) {
            logActivity(`❌ Access denied: Cannot view ${docName}`, 'error');
            showAccessDenied(`You don't have permission to view this document.`);
            return;
        }
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        const data = await response.json();
        logActivity(`📖 Opened document: ${docName} (${data.canEdit ? 'edit' : 'read'} access)`, 'success');
        showDocumentContent(docName, data.content, false, data.canEdit ? 'editor' : 'viewer');
        
    } catch (error) {
        logActivity(`Error opening document: ${error.message}`, 'error');
        showAccessDenied(`Error loading document: ${error.message}`);
    }
}

async function editDocument(docName) {
    try {
        // Debug: Check what we received
        console.log('DEBUG: editDocument called with:', docName, typeof docName);
        
        // Ensure docName is a string
        if (typeof docName !== 'string') {
            console.error('ERROR: Document name is not a string:', docName);
            logActivity(`❌ Error: Invalid document name format`, 'error');
            return;
        }
        
        logActivity(`✏️ Opening document for editing: ${docName}...`, 'info');
        
        const response = await fetch(`${AUTH_BASE_URL}/documents/${docName}`, {
            method: 'GET',
            credentials: 'include'
        });
        
        if (response.status === 403) {
            logActivity(`❌ Access denied: Cannot edit ${docName}`, 'error');
            showAccessDenied(`You don't have permission to edit this document.`);
            return;
        }
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        const data = await response.json();
        
        // Check if user can edit
        if (!data.canEdit) {
            logActivity(`❌ Access denied: Cannot edit ${docName} (read-only access)`, 'error');
            showAccessDenied(`You don't have permission to edit this document. You have read-only access.`);
            return;
        }
        
        logActivity(`✏️ Editing document: ${docName} (edit access)`, 'success');
        showDocumentContent(docName, data.content, true, 'editor');
        
    } catch (error) {
        logActivity(`Error editing document: ${error.message}`, 'error');
        showAccessDenied(`Error loading document for editing: ${error.message}`);
    }
}

function shareDocument(docName) {
    // Debug: Check what we received
    console.log('DEBUG: shareDocument called with:', docName, typeof docName);
    
    // Ensure docName is a string
    if (typeof docName !== 'string') {
        console.error('ERROR: Document name is not a string:', docName);
        logActivity(`❌ Error: Invalid document name format`, 'error');
        return;
    }
    
    // Check if user is owner by checking their permissions on this specific document
    // This is more accurate than checking the global role
    if (!currentUser) {
        logActivity(`❌ Access denied: Please log in first`, 'error');
        return;
    }
    
    // For now, allow any authenticated user to try sharing - the backend will enforce proper authorization
    // The Mini-Zanzibar service will check if the user actually owns the document
    logActivity(`📤 Opening share dialog for ${docName} (authorization will be checked by backend)`, 'info');
    showShareModal(docName);
}

function showShareModal(docName) {
    const modal = document.createElement('div');
    modal.className = 'modal';
    modal.style.cssText = `
        position: fixed;
        top: 0;
        left: 0;
        width: 100%;
        height: 100%;
        background-color: rgba(0,0,0,0.5);
        display: flex;
        justify-content: center;
        align-items: center;
        z-index: 1000;
    `;
    
    modal.innerHTML = `
        <div class="modal-content" style="
            background: white;
            padding: 20px;
            border-radius: 8px;
            max-width: 500px;
            width: 90%;
        ">
            <div class="modal-header" style="
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 20px;
                border-bottom: 1px solid #ddd;
                padding-bottom: 10px;
            ">
                <h3 style="margin: 0;">📤 Share Document: ${docName}</h3>
                <button onclick="this.closest('.modal').remove()" class="close-btn" style="
                    background: none;
                    border: none;
                    font-size: 24px;
                    cursor: pointer;
                    color: #999;
                ">&times;</button>
            </div>
            <div class="modal-body" style="margin-bottom: 20px;">
                <form id="share-form" style="display: flex; flex-direction: column; gap: 15px;">
                    <div>
                        <label for="share-user" style="display: block; margin-bottom: 5px; font-weight: bold;">
                            Share with user:
                        </label>
                        <input type="text" id="share-user" placeholder="Enter username (e.g., bob)" 
                               style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;" required>
                    </div>
                    <div>
                        <label for="share-permission" style="display: block; margin-bottom: 5px; font-weight: bold;">
                            Permission level:
                        </label>
                        <select id="share-permission" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
                            <option value="viewer">Viewer (can read only)</option>
                            <option value="editor">Editor (can read and write)</option>
                        </select>
                    </div>
                    <div style="background-color: #f8f9fa; padding: 10px; border-radius: 4px; font-size: 14px;">
                        <strong>Note:</strong> This will create an ACL entry allowing the specified user to access this document 
                        with the selected permission level through the Zanzibar authorization system.
                    </div>
                </form>
            </div>
            <div class="modal-footer" style="
                display: flex;
                gap: 10px;
                justify-content: flex-end;
                border-top: 1px solid #ddd;
                padding-top: 15px;
            ">
                <button onclick="shareDocumentWithUser('${docName}')" class="btn-primary" style="
                    background-color: #007bff;
                    color: white;
                    border: none;
                    padding: 10px 20px;
                    border-radius: 4px;
                    cursor: pointer;
                ">📤 Share Document</button>
                <button onclick="this.closest('.modal').remove()" class="btn-secondary" style="
                    background-color: #6c757d;
                    color: white;
                    border: none;
                    padding: 10px 20px;
                    border-radius: 4px;
                    cursor: pointer;
                ">Cancel</button>
            </div>
        </div>
    `;
    
    document.body.appendChild(modal);
    
    // Focus the username input
    setTimeout(() => {
        const userInput = document.getElementById('share-user');
        if (userInput) userInput.focus();
    }, 100);
}

async function shareDocumentWithUser(docName) {
    const userInput = document.getElementById('share-user');
    const permissionSelect = document.getElementById('share-permission');
    
    if (!userInput || !permissionSelect) return;
    
    const username = userInput.value.trim();
    const permission = permissionSelect.value;
    
    if (!username) {
        alert('Please enter a username');
        return;
    }
    
    try {
        logActivity(`📤 Sharing ${docName} with ${username} as ${permission}...`, 'info');
        
        // Create ACL entry using the Mini-Zanzibar API
        const aclData = {
            object: `doc:${docName}`,
            relation: permission,
            user: `user:${username}`
        };
        
        const response = await makeApiCall('/acl', 'POST', aclData);
        
        logActivity(`✅ Successfully shared ${docName} with ${username} as ${permission}`, 'success');
        
        // Close the modal
        const modal = userInput.closest('.modal');
        if (modal) modal.remove();
        
        // Automatically refresh the documents list and permissions
        loadDocuments();
        if (document.getElementById('test-authorization').style.display !== 'none') {
            refreshMyPermissions();
        }
        
        // Show success message
        const successModal = document.createElement('div');
        successModal.className = 'modal';
        successModal.style.cssText = `
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background-color: rgba(0,0,0,0.5);
            display: flex;
            justify-content: center;
            align-items: center;
            z-index: 1000;
        `;
        
        successModal.innerHTML = `
            <div class="modal-content" style="
                background: white;
                padding: 30px;
                border-radius: 8px;
                text-align: center;
                max-width: 400px;
            ">
                <h3 style="color: #28a745; margin: 0 0 15px 0;">✅ Document Shared Successfully!</h3>
                <p style="margin: 0 0 20px 0;">
                    <strong>${username}</strong> now has <strong>${permission}</strong> access to <strong>${docName}</strong>
                </p>
                <button onclick="this.closest('.modal').remove()" style="
                    background-color: #28a745;
                    color: white;
                    border: none;
                    padding: 10px 20px;
                    border-radius: 4px;
                    cursor: pointer;
                ">OK</button>
            </div>
        `;
        
        document.body.appendChild(successModal);
        
        // Auto-close after 3 seconds
        setTimeout(() => {
            if (successModal.parentNode) {
                successModal.remove();
            }
        }, 3000);
        
    } catch (error) {
        logActivity(`❌ Failed to share ${docName}: ${error.message}`, 'error');
        alert(`Failed to share document: ${error.message}`);
    }
}

// Document display functions
function showDocumentContent(docName, content, isEditing = false, permission = 'viewer') {
    const modal = document.createElement('div');
    modal.className = 'modal';
    modal.style.cssText = `
        position: fixed;
        top: 0;
        left: 0;
        width: 100%;
        height: 100%;
        background-color: rgba(0,0,0,0.5);
        display: flex;
        justify-content: center;
        align-items: center;
        z-index: 1000;
    `;
    
    modal.innerHTML = `
        <div class="modal-content" style="
            background: white;
            padding: 20px;
            border-radius: 8px;
            max-width: 80%;
            max-height: 80%;
            overflow-y: auto;
            position: relative;
        ">
            <div class="modal-header" style="
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 15px;
                border-bottom: 1px solid #ddd;
                padding-bottom: 10px;
            ">
                <div>
                    <h3 style="margin: 0;">${isEditing ? 'Editing' : 'Viewing'}: ${docName}</h3>
                    <span class="permission-indicator" style="
                        background-color: #e9ecef;
                        padding: 2px 8px;
                        border-radius: 4px;
                        font-size: 12px;
                        color: #6c757d;
                    ">Permission: ${permission}</span>
                </div>
                <button onclick="this.closest('.modal').remove()" class="close-btn" style="
                    background: none;
                    border: none;
                    font-size: 24px;
                    cursor: pointer;
                    color: #999;
                ">&times;</button>
            </div>
            <div class="modal-body" style="margin-bottom: 15px;">
                ${isEditing ? 
                    `<textarea id="doc-editor" rows="15" style="
                        width: 100%;
                        font-family: monospace;
                        border: 1px solid #ddd;
                        padding: 10px;
                        border-radius: 4px;
                    ">${content}</textarea>` : 
                    `<div class="document-content" style="
                        line-height: 1.6;
                        font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                    ">${formatMarkdownContent(content)}</div>`
                }
            </div>
            <div class="modal-footer" style="
                display: flex;
                gap: 10px;
                justify-content: flex-end;
                border-top: 1px solid #ddd;
                padding-top: 10px;
            ">
                ${isEditing ? 
                    `<button onclick="saveDocument('${docName}')" class="btn-primary" style="
                        background-color: #007bff;
                        color: white;
                        border: none;
                        padding: 8px 16px;
                        border-radius: 4px;
                        cursor: pointer;
                    ">💾 Save</button>` : 
                    ''
                }
                <button onclick="this.closest('.modal').remove()" class="btn-secondary" style="
                    background-color: #6c757d;
                    color: white;
                    border: none;
                    padding: 8px 16px;
                    border-radius: 4px;
                    cursor: pointer;
                ">Close</button>
            </div>
        </div>
    `;
    
    document.body.appendChild(modal);
    
    // Focus the textarea if editing
    if (isEditing) {
        setTimeout(() => {
            const textarea = document.getElementById('doc-editor');
            if (textarea) textarea.focus();
        }, 100);
    }
}

function showAccessDenied(message = 'Access denied') {
    const modal = document.createElement('div');
    modal.className = 'modal';
    modal.style.cssText = `
        position: fixed;
        top: 0;
        left: 0;
        width: 100%;
        height: 100%;
        background-color: rgba(0,0,0,0.5);
        display: flex;
        justify-content: center;
        align-items: center;
        z-index: 1000;
    `;
    
    modal.innerHTML = `
        <div class="modal-content" style="
            background: white;
            padding: 20px;
            border-radius: 8px;
            max-width: 500px;
            text-align: center;
        ">
            <div class="modal-header" style="margin-bottom: 15px;">
                <h3 style="color: #dc3545; margin: 0;">❌ Access Denied</h3>
            </div>
            <div class="modal-body" style="margin-bottom: 15px;">
                <p>${message}</p>
            </div>
            <div class="modal-footer">
                <button onclick="this.closest('.modal').remove()" class="btn-secondary" style="
                    background-color: #6c757d;
                    color: white;
                    border: none;
                    padding: 8px 16px;
                    border-radius: 4px;
                    cursor: pointer;
                ">Close</button>
            </div>
        </div>
    `;
    
    document.body.appendChild(modal);
}

function formatMarkdownContent(content) {
    return content
        .replace(/^# (.*$)/gim, '<h1 style="color: #333; border-bottom: 2px solid #eee; padding-bottom: 10px;">$1</h1>')
        .replace(/^## (.*$)/gim, '<h2 style="color: #555; border-bottom: 1px solid #eee; padding-bottom: 5px;">$1</h2>')
        .replace(/^### (.*$)/gim, '<h3 style="color: #666;">$1</h3>')
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.*?)\*/g, '<em>$1</em>')
        .replace(/\n/g, '<br>');
}

async function saveDocument(docName) {
    const textarea = document.getElementById('doc-editor');
    if (!textarea) return;
    
    const content = textarea.value;
    
    try {
        logActivity(`💾 Saving document: ${docName}...`, 'info');
        
        const response = await fetch(`${AUTH_BASE_URL}/documents/${docName}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            credentials: 'include',
            body: JSON.stringify({ content: content })
        });
        
        const result = await response.json();
        
        if (response.ok) {
            logActivity(`✅ Document saved: ${docName}`, 'success');
            
            // Close the modal
            const modal = textarea.closest('.modal');
            if (modal) modal.remove();
            
            // Refresh the document list
            loadDocuments();
        } else {
            logActivity(`❌ Failed to save document: ${result.error}`, 'error');
        }
        
    } catch (error) {
        logActivity(`❌ Failed to save document: ${error.message}`, 'error');
    }
}

// Authorization Testing
// Refresh current user's permissions overview
async function refreshMyPermissions() {
    if (!currentUser) {
        showTestError('Please log in first');
        return;
    }

    console.log('DEBUG: Current user object:', currentUser);
    const permissionsGrid = document.getElementById('user-permissions');
    permissionsGrid.innerHTML = '<div>Loading permissions...</div>';

    try {
        // Get the list of documents from the server
        const documentsResponse = await fetch(`${AUTH_BASE_URL}/documents`, {
            method: 'GET',
            credentials: 'include'
        });
        
        if (!documentsResponse.ok) {
            throw new Error('Failed to load documents list');
        }
        
        const documentsData = await documentsResponse.json();
        console.log('DEBUG: Documents data for permissions:', documentsData);
        
        const documents = documentsData.documents || [];
        console.log('DEBUG: Documents array:', documents);
        
        const permissions = ['viewer', 'editor', 'owner'];
        let permissionCards = '';

        for (const doc of documents) {
            console.log('DEBUG: Processing permissions for doc:', doc, typeof doc);
            
            let userPermissions = [];
            
            // Handle both old format (strings) and new format (objects) 
            let docName;
            if (typeof doc === 'string') {
                docName = doc;
            } else if (typeof doc === 'object' && doc.name) {
                docName = doc.name;
            } else {
                console.error('ERROR: Invalid document format for permissions:', doc);
                continue; // Skip invalid documents
            }
            
            console.log(`DEBUG: Processing permissions for document: ${docName}`);
            
            // If we have detailed permissions from the backend, use them directly
            if (typeof doc === 'object' && doc.canView !== undefined) {
                console.log(`DEBUG: Using backend permissions for ${docName}:`, {canView: doc.canView, canEdit: doc.canEdit, canOwn: doc.canOwn});
                if (doc.canOwn) userPermissions.push('owner');
                if (doc.canEdit) userPermissions.push('editor');
                if (doc.canView) userPermissions.push('viewer');
            } else {
                console.log(`DEBUG: Falling back to API calls for ${docName}`);
                // Fallback to API calls for permission checking
                for (const permission of permissions) {
                    try {
                        // Handle username format - ensure it starts with user:
                        const userId = currentUser.username.startsWith('user:') ? currentUser.username : `user:${currentUser.username}`;
                        const response = await makeApiCall(`/acl/check?object=doc:${docName}&relation=${permission}&user=${userId}`);
                        if (response.authorized) {
                            userPermissions.push(permission);
                        }
                    } catch (error) {
                        console.error(`Error checking ${permission} for ${docName}:`, error);
                    }
                }
            }

            const highestPermission = userPermissions.includes('owner') ? 'owner' : 
                                    userPermissions.includes('editor') ? 'editor' : 
                                    userPermissions.includes('viewer') ? 'viewer' : 'none';

            // Display name without .md extension for cleaner UI
            const displayName = docName.replace('.md', '');
            
            permissionCards += `
                <div class="permission-card">
                    <h4>${displayName}</h4>
                    <div class="permission-badge ${highestPermission}">
                        ${highestPermission === 'none' ? 'No Access' : highestPermission.charAt(0).toUpperCase() + highestPermission.slice(1)}
                    </div>
                    ${userPermissions.length > 1 ? `<div style="font-size: 12px; color: #666; margin-top: 5px;">Also: ${userPermissions.filter(p => p !== highestPermission).join(', ')}</div>` : ''}
                </div>
            `;
        }

        permissionsGrid.innerHTML = permissionCards || '<div>No documents found</div>';
        logActivity(`🔍 Refreshed permissions for ${currentUser.username} (${documents.length} documents)`, 'info');

    } catch (error) {
        permissionsGrid.innerHTML = '<div class="error">Error loading permissions</div>';
        showTestError(`Error loading permissions: ${error.message}`);
    }
}

// Initialize test authorization section when it's shown
function initializeTestAuthSection() {
    if (currentUser) {
        // Pre-fill user field with current user
        document.getElementById('test-user').value = currentUser.username;
        // Load documents into dropdown
        loadDocumentsForTesting();
        // Auto-load current user's permissions
        refreshMyPermissions();
    }
}

// Load documents into the test authorization dropdown
async function loadDocumentsForTesting() {
    try {
        const response = await fetch(`${AUTH_BASE_URL}/documents`, {
            method: 'GET',
            credentials: 'include'
        });
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        const data = await response.json();
        console.log('DEBUG: Loading documents for testing:', data);
        
        const dropdown = document.getElementById('test-document');
        
        if (dropdown && data.documents && data.documents.length > 0) {
            // Clear existing options except the first one
            dropdown.innerHTML = '<option value="">Select a document...</option>';
            
            // Add documents to dropdown (remove .md extension for display)
            data.documents.forEach(doc => {
                console.log('DEBUG: Processing test document:', doc, typeof doc);
                
                const option = document.createElement('option');
                // Handle both old format (strings) and new format (objects)
                let docName;
                if (typeof doc === 'string') {
                    docName = doc;
                } else if (typeof doc === 'object' && doc.name) {
                    docName = doc.name;
                } else {
                    console.error('ERROR: Invalid document format for testing:', doc);
                    return; // Skip invalid documents
                }
                
                // Use the full filename as value (for API calls)
                option.value = docName;
                // Display name without .md extension for cleaner UI
                option.textContent = docName.replace('.md', '');
                dropdown.appendChild(option);
            });
            
            logActivity(`🔄 Loaded ${data.documents.length} documents into test dropdown`, 'info');
        }
    } catch (error) {
        console.error('Error loading documents for testing:', error);
        logActivity(`❌ Failed to load documents for testing: ${error.message}`, 'error');
    }
}

async function testAuthorization() {
    let user = document.getElementById('test-user').value.trim();
    const documentName = document.getElementById('test-document').value.trim();
    const permission = document.getElementById('test-permission').value;
    
    // If no user specified, use current user
    if (!user && currentUser) {
        user = currentUser.username;
        document.getElementById('test-user').value = user;
    }
    
    if (!user || !documentName) {
        showTestError('Please select a user and document');
        return;
    }
    
    try {
        // Handle username format - avoid double "user:" prefix
        const userId = user.startsWith('user:') ? user : `user:${user}`;
        
        const response = await makeApiCall(`/acl/check?object=doc:${documentName}&relation=${permission}&user=${userId}`);
        
        const resultText = response.authorized ? 
            `✅ AUTHORIZED: ${user} has ${permission} access to ${documentName}` :
            `❌ DENIED: ${user} does NOT have ${permission} access to ${documentName}`;
        
        const resultClass = response.authorized ? 'result-success' : 'result-error';
        
        showTestResult(resultText, resultClass);
        logActivity(`🔐 Authorization test: ${user} -> ${documentName} (${permission}): ${response.authorized ? 'ALLOWED' : 'DENIED'}`, response.authorized ? 'success' : 'error');
        
    } catch (error) {
        showTestResult(`Error: ${error.message}`, 'result-error');
        logActivity(`🔐 Authorization test error: ${error.message}`, 'error');
    }
}

// ACL Management (Owner only)
async function createACL() {
    // Remove hardcoded role check - let the backend enforce proper authorization
    // The Mini-Zanzibar service will check if the user actually has permission to create ACLs
    if (!currentUser) {
        logActivity(`❌ Access denied: Please log in first`, 'error');
        return;
    }
    
    const object = document.getElementById('acl-object').value.trim();
    const relation = document.getElementById('acl-relation').value.trim();
    const user = document.getElementById('acl-user').value.trim();
    
    if (!object || !relation || !user) {
        showTestError('Please fill in all ACL fields');
        return;
    }
    
    try {
        const response = await makeApiCall('/acl', 'POST', {
            object: object,
            relation: relation,
            user: user
        });
        
        logActivity(`✅ ACL created: ${object}#${relation}@${user}`, 'success');
        clearACLForm();
    } catch (error) {
        logActivity(`❌ Failed to create ACL: ${error.message}`, 'error');
    }
}

// Utility functions
function showSection(sectionName) {
    const sections = ['documents', 'access-control', 'test-authorization'];
    sections.forEach(section => {
        const element = document.getElementById(section);
        if (element) {
            element.style.display = 'none';
        }
    });
    
    const targetSection = document.getElementById(sectionName);
    if (targetSection) {
        targetSection.style.display = 'block';
    }
    
    // Initialize specific sections when shown
    if (sectionName === 'test-authorization') {
        initializeTestAuthSection();
    }
    
    updateNavigationState(sectionName);
}

function updateNavigationState(activeSection) {
    const navButtons = document.querySelectorAll('.nav-btn');
    navButtons.forEach(btn => {
        btn.classList.remove('active');
    });
    
    const activeBtn = document.getElementById(`nav-${activeSection}`);
    if (activeBtn) {
        activeBtn.classList.add('active');
    }
}

function logActivity(message, type = 'info') {
    const timestamp = new Date().toLocaleTimeString();
    const logEntry = document.createElement('div');
    logEntry.className = `log-entry log-${type}`;
    logEntry.innerHTML = `<span class="timestamp">[${timestamp}]</span> ${message}`;
    
    const logContainer = document.getElementById('activity-log');
    if (logContainer) {
        logContainer.appendChild(logEntry);
        logContainer.scrollTop = logContainer.scrollHeight;
    }
    
    console.log(`[${timestamp}] ${message}`);
}

function updateConnectionStatus(isConnected) {
    const statusIndicator = document.createElement('span');
    statusIndicator.className = `status-indicator ${isConnected ? 'status-online' : 'status-offline'}`;
    
    const statusText = document.createElement('span');
    statusText.textContent = isConnected ? 'Connected to Auth Service' : 'Auth Service Offline';
    
    const header = document.querySelector('header p');
    if (header) {
        header.innerHTML = '';
        header.appendChild(statusIndicator);
        header.appendChild(statusText);
    }
}

function showTestResult(message, className) {
    const resultDiv = document.getElementById('test-result');
    if (resultDiv) {
        resultDiv.textContent = message;
        resultDiv.className = className;
        resultDiv.style.display = 'block';
    }
}

function showTestError(message) {
    const errorDiv = document.getElementById('test-error');
    if (errorDiv) {
        errorDiv.textContent = message;
        errorDiv.style.display = 'block';
    }
}

function clearACLForm() {
    document.getElementById('acl-object').value = '';
    document.getElementById('acl-relation').value = '';
    document.getElementById('acl-user').value = '';
}

// Show update notification
function showUpdateNotification(message) {
    // Create notification element
    const notification = document.createElement('div');
    notification.style.cssText = `
        position: fixed;
        top: 20px;
        right: 20px;
        background-color: #28a745;
        color: white;
        padding: 15px 20px;
        border-radius: 5px;
        box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        z-index: 2000;
        max-width: 400px;
        animation: slideInRight 0.3s ease-out;
    `;
    
    notification.innerHTML = `
        <div style="display: flex; justify-content: space-between; align-items: center;">
            <span>📄 ${message}</span>
            <button onclick="this.parentElement.parentElement.remove()" style="
                background: none;
                border: none;
                color: white;
                font-size: 18px;
                cursor: pointer;
                margin-left: 10px;
            ">&times;</button>
        </div>
    `;
    
    // Add animation styles
    const style = document.createElement('style');
    style.textContent = `
        @keyframes slideInRight {
            from { transform: translateX(100%); opacity: 0; }
            to { transform: translateX(0); opacity: 1; }
        }
    `;
    document.head.appendChild(style);
    
    document.body.appendChild(notification);
    
    // Auto-remove after 5 seconds
    setTimeout(() => {
        if (notification.parentNode) {
            notification.style.animation = 'slideInRight 0.3s ease-out reverse';
            setTimeout(() => {
                if (notification.parentNode) {
                    notification.remove();
                }
            }, 300);
        }
    }, 5000);
}