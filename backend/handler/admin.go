package handler

import (
    "net/http"
    "strings"
    "github.com/gin-gonic/gin"
    "GO_C2/utils"
    "GO_C2/db"
    "golang.org/x/crypto/bcrypt"
)

func AuthRequired() gin.HandlerFunc {
    return func(c *gin.Context) {
        auth := c.GetHeader("Authorization")
        if auth == "" {
            auth, _ = c.Cookie("token")
            if auth == "" { c.JSON(http.StatusUnauthorized, gin.H{"error":"Unauthorized"}); c.Abort(); return }
        } else {
            parts := strings.SplitN(auth, " ", 2)
            if !(len(parts) == 2 && parts[0] == "Bearer") { c.JSON(http.StatusUnauthorized, gin.H{"error":"Invalid token format"}); c.Abort(); return }
            auth = parts[1]
        }
        claims, err := utils.ParseToken(auth)
        if err != nil { c.JSON(http.StatusUnauthorized, gin.H{"error":"Invalid token"}); c.Abort(); return }
        c.Set("username", claims.Username)
        c.Next()
    }
}

func AdminLoginHandler(c *gin.Context) {
    var login struct { Username string `json:"username"`; Password string `json:"password"` }
    if err := c.BindJSON(&login); err != nil { c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid login data"}); return }
    user, err := db.GetUserByUsername(login.Username)
    if err != nil { c.JSON(http.StatusUnauthorized, gin.H{"error":"Invalid credentials"}); return }
    if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(login.Password)); err != nil { c.JSON(http.StatusUnauthorized, gin.H{"error":"Invalid credentials"}); return }
    token, err := utils.GenerateToken(login.Username)
    if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error":"Failed to generate token"}); return }
    c.SetCookie("token", token, 86400, "/", "", false, true)
    c.JSON(http.StatusOK, gin.H{"status":"success","token":token})
}

// 用户管理 CRUD
func ListUsersHandler(c *gin.Context) {
    users, err := db.ListUsers(); if err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error":"Failed to list users"}); return }
    // 不返回密码哈希
    type u struct { ID int64 `json:"id"`; Username string `json:"username"` }
    resp := make([]u, 0, len(users))
    for _, item := range users { resp = append(resp, u{ID: item.ID, Username: item.Username}) }
    c.JSON(http.StatusOK, gin.H{"status":"success","data": resp})
}

func CreateUserHandler(c *gin.Context) {
    var body struct{ Username string `json:"username"`; Password string `json:"password"` }
    if err := c.BindJSON(&body); err != nil || body.Username == "" || body.Password == "" { c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid body"}); return }
    if err := db.CreateUser(body.Username, body.Password); err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error":"Failed to create user"}); return }
    c.JSON(http.StatusOK, gin.H{"status":"success"})
}

func UpdateUserPasswordHandler(c *gin.Context) {
    var body struct{ Username string `json:"username"`; Password string `json:"password"`; OldPassword string `json:"old_password"` }
    if err := c.BindJSON(&body); err != nil || body.Username == "" || body.Password == "" { c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid body"}); return }
    user, err := db.GetUserByUsername(body.Username)
    if err != nil { c.JSON(http.StatusUnauthorized, gin.H{"error":"Invalid user"}); return }
    if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.OldPassword)); err != nil { c.JSON(http.StatusUnauthorized, gin.H{"error":"Old password incorrect"}); return }
    if err := db.UpdateUserPassword(body.Username, body.Password); err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error":"Failed to update password"}); return }
    c.JSON(http.StatusOK, gin.H{"status":"success"})
}

func DeleteUserHandler(c *gin.Context) {
    var body struct{ Username string `json:"username"` }
    if err := c.BindJSON(&body); err != nil || body.Username == "" { c.JSON(http.StatusBadRequest, gin.H{"error":"Invalid body"}); return }
    if err := db.DeleteUser(body.Username); err != nil { c.JSON(http.StatusInternalServerError, gin.H{"error":"Failed to delete user"}); return }
    c.JSON(http.StatusOK, gin.H{"status":"success"})
}


