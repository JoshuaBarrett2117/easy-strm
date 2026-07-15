package service

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	"github.com/golang-jwt/jwt/v5"
)

type AuthService struct {
	userDAO   *dao.UserDAO
	jwtSecret string
}

func NewAuthService(userDAO *dao.UserDAO, jwtSecret string) *AuthService {
	return &AuthService{
		userDAO:   userDAO,
		jwtSecret: jwtSecret,
	}
}

type JWTClaims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

func md5Hash(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *AuthService) GenerateToken(userID int) (string, error) {
	claims := JWTClaims{
		UserID: int64(userID),
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

func (s *AuthService) VerifyToken(tokenString string) (*JWTClaims, error) {
	logger.Debugf("VerifyToken 被调用, tokenString长度: %d", len(tokenString))
	logger.Debugf("使用JWT secret: %s (长度: %d)", s.jwtSecret, len(s.jwtSecret))

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		logger.Warnf("VerifyToken 解析失败: %T - %v", err, err)
		return nil, fmt.Errorf("AuthService[VerifyToken] 解析token失败: %v", err)
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		logger.Debugf("VerifyToken 成功, userID: %v", claims.UserID)
		return claims, nil
	}
	logger.Warnf("VerifyToken token.Valid=%v", token.Valid)
	return nil, fmt.Errorf("AuthService[VerifyToken] 无效的token")
}

func (s *AuthService) Login(name, password string) (*domain.User, string, error) {
	user, err := s.userDAO.GetByName(name)
	if err != nil {
		logger.Errorf("AuthService[Login] 获取用户失败: %v", err)
		return nil, "", fmt.Errorf("用户不存在")
	}
	if user == nil {
		return nil, "", fmt.Errorf("用户不存在")
	}

	// 前端已经对密码进行了MD5哈希，直接比较即可，避免双重哈希
	if user.Password != password {
		logger.Warnf("AuthService[Login] 密码不匹配: 输入=%s, 数据库=%s", password, user.Password)
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

func (s *AuthService) GetUserByID(id int) (*domain.User, error) {
	logger.Debugf("AuthService[GetUserByID] 开始查询用户ID: %d", id)
	user, err := s.userDAO.GetByID(id)
	if err != nil {
		logger.Errorf("AuthService[GetUserByID] 查询用户失败: %v", err)
		return nil, err
	}
	if user == nil {
		logger.Warnf("AuthService[GetUserByID] 用户不存在, id: %d", id)
		return nil, nil
	}
	logger.Debugf("AuthService[GetUserByID] 查询成功: %+v", user)
	return user, nil
}
func (s *AuthService) UpdatePassword(name, newPassword string) error {
	newHash := md5Hash(newPassword)
	return s.userDAO.UpdatePassword(name, newHash)
}
