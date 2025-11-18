package utils

import (
    "time"
    "github.com/golang-jwt/jwt/v5"
    "GO_C2/config"
)

type Claims struct {
    Username string `json:"username"`
    jwt.RegisteredClaims
}

func getJWTSecret() []byte { return []byte(config.GlobalConfig.JWTSecret) }

func GenerateToken(username string) (string, error) {
    claims := Claims{username, jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
        IssuedAt:  jwt.NewNumericDate(time.Now()),
        NotBefore: jwt.NewNumericDate(time.Now()),
    }}
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(getJWTSecret())
}

func ParseToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
        return getJWTSecret(), nil
    })
    if err != nil { return nil, err }
    if claims, ok := token.Claims.(*Claims); ok && token.Valid { return claims, nil }
    return nil, jwt.ErrSignatureInvalid
}


