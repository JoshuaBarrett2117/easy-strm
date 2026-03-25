package service

import (
	"fmt"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	"github.com/golang-jwt/jwt/v5"
)

type AuthService struct {
	userDAO     *dao.UserDAO
	jwtSecret   string
}

func NewAuthService(userDAO *dao.UserDAO, jwtSecret string) *AuthService {
	return &AuthService{
		userDAO:   userDAO,
		jwtSecret: jwtSecret,
	}
}

type JWTClaims struct {
	UserID int `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT token
func (s *AuthService) GenerateToken(userID int) (string, error) {
	claims := JWTClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("AuthService[GenerateToken] 生成token失败: %v", err)
	}
	return tokenString, nil
}

// VerifyToken 验证JWT token
func (s *AuthService) VerifyToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("AuthService[VerifyToken] 解析token失败: %v", err)
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("AuthService[VerifyToken] 无效的token")
}

// Login 用户登录
func (s *AuthService) Login(name, password string) (*domain.User, string, error) {
	user, err := s.userDAO.GetByName(name)
	if err != nil {
		logger.Errorf("AuthService[Login] 获取用户失败: %v", err)
		return nil, "", fmt.Errorf("用户不存在")
	}
	if user == nil {
		return nil, "", fmt.Errorf("用户不存在")
	}

	if user.Password != password {
		return nil, "", fmt.Errorf("密码错误")
	}

	token, err := s.GenerateToken(user.ID)
	if err != nil {
		logger.Errorf("AuthService[Login] 生成token失败: %v", err)
		return nil, "", fmt.Errorf("生成token失败")
	}

	logger.Infof("AuthService[Login] 用户登录成功: %s", name)
	return user, token, nil
}

// GetUserByID 根据ID获取用户
func (s *AuthService) GetUserByID(id int) (*domain.User, error) {
	return s.userDAO.GetByID(id)
}

// SeedAdmin 确保admin用户存在
func (s *AuthService) SeedAdmin() error {
	const adminName = "admin"
	const adminPasswordMD5 = "21232f297a57a5a743894a0e4a801fc3"

	user, err := s.userDAO.GetByName(adminName)
	if err == nil && user != nil {
		logger.Infof("AuthService[SeedAdmin] Admin用户已存在")
		return nil
	}

	_, err = s.userDAO.Create(adminName, adminPasswordMD5)
	if err != nil {
		return fmt.Errorf("AuthService[SeedAdmin] 创建admin用户失败: %v", err)
	}

	logger.Infof("AuthService[SeedAdmin] Admin用户创建成功")
	return nil
}
