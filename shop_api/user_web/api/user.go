package api

import (
	"context"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"net/http"
	"shop_api/user_web/forms"
	"shop_api/user_web/global"
	reponse "shop_api/user_web/global/response"
	"shop_api/user_web/middlewares"
	"shop_api/user_web/models"
	"shop_api/user_web/proto"
	"strconv"
	"strings"
	"time"
)

func removeTopStruct(fileds map[string]string) map[string]string {
	rsp := map[string]string{}
	for field, err := range fileds {
		rsp[field[strings.Index(field, ".")+1:]] = err
	}
	return rsp
}

func HandleGrpcErrorToHttp(err error, c *gin.Context) {
	//将grpc的code转换成http的状态码
	if err != nil {
		if e, ok := status.FromError(err); ok {
			switch e.Code() {
			case codes.NotFound:
				c.JSON(http.StatusNotFound, gin.H{
					"msg": e.Message(),
				})
			case codes.Internal:
				c.JSON(http.StatusInternalServerError, gin.H{
					"msg:": "内部错误",
				})
			case codes.InvalidArgument:
				c.JSON(http.StatusBadRequest, gin.H{
					"msg": "参数错误",
				})
			case codes.Unavailable:
				c.JSON(http.StatusInternalServerError, gin.H{
					"msg": "用户服务不可用",
				})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{
					"msg": e.Code(),
				})
			}
			return
		}
	}
}

func HandleValidatorError(c *gin.Context, err error) {
	errs, ok := err.(validator.ValidationErrors)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"msg": err.Error(),
		})
	}
	c.JSON(http.StatusBadRequest, gin.H{
		"error": removeTopStruct(errs.Translate(global.Trans)),
	})
	return
}

func GetUserList(ctx *gin.Context) {
	//拨号连接用户grpc服务器 跨域的问题 - 后端解决 也可以前端来解决
	claims, _ := ctx.Get("claims")
	currentUser := claims.(*models.CustomClaims)
	zap.S().Infof("访问用户: %d", currentUser.ID)
	//生成grpc的client并调用接口

	pn := ctx.DefaultQuery("pn", "0")
	pnInt, _ := strconv.Atoi(pn)
	pSize := ctx.DefaultQuery("psize", "10")
	pSizeInt, _ := strconv.Atoi(pSize)

	rsp, err := global.UserSrvClient.GetUserList(context.Background(), &proto.PageInfo{
		Pn:    uint32(pnInt),
		PSize: uint32(pSizeInt),
	})
	if err != nil {
		zap.S().Errorw("[GetUserList] 查询 【用户列表】失败")
		HandleGrpcErrorToHttp(err, ctx)
		return
	}

	reMap := gin.H{
		"total": rsp.Total,
	}
	result := make([]interface{}, 0)
	for _, value := range rsp.Data {
		user := reponse.UserResponse{
			Id:       value.Id,
			NickName: value.NickName,
			//Birthday: time.Time(time.Unix(int64(value.BirthDay), 0)).Format("2006-01-02"),
			Birthday: reponse.JsonTime(time.Unix(int64(value.BirthDay), 0)),
			Gender:   value.Gender,
			Mobile:   value.Mobile,
		}
		result = append(result, user)
	}

	reMap["data"] = result
	ctx.JSON(http.StatusOK, reMap)
}

func PassWordLogin(ctx *gin.Context) {
	//表单验证
	passwordLoginForm := forms.PassWordLoginForm{}
	if err := ctx.ShouldBind(&passwordLoginForm); err != nil {
		HandleValidatorError(ctx, err)
		return
	}

	if store.Verify(passwordLoginForm.CaptchaId, passwordLoginForm.Captcha, false) {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"captcha": "验证码错误",
		})
		return
	}

	//登录的逻辑
	if rsp, err := global.UserSrvClient.GetUserByMobile(context.Background(), &proto.MobileRequest{
		Mobile: passwordLoginForm.Mobile,
	}); err != nil {
		if e, ok := status.FromError(err); ok {
			switch e.Code() {
			case codes.NotFound:
				ctx.JSON(http.StatusBadRequest, map[string]string{
					"mobile": "用户不存在",
				})
			default:
				ctx.JSON(http.StatusInternalServerError, map[string]string{
					"mobile": "登录失败",
				})
			}
			return
		}
	} else {
		//只是查询到用户了而已，并没有检查密码
		if passRsp, pasErr := global.UserSrvClient.CheckPassWord(context.Background(), &proto.PasswordCheckInfo{
			Password:          passwordLoginForm.PassWord,
			EncryptedPassword: rsp.PassWord,
		}); pasErr != nil {
			ctx.JSON(http.StatusInternalServerError, map[string]string{
				"password": "登录失败",
			})
		} else {
			if passRsp.Success {
				//生成token
				j := middlewares.NewJWT()
				claims := models.CustomClaims{
					ID:          uint(rsp.Id),
					NickName:    rsp.NickName,
					AuthorityId: uint(rsp.Role),
					StandardClaims: jwt.StandardClaims{
						NotBefore: time.Now().Unix(),               //签名的生效时间
						ExpiresAt: time.Now().Unix() + 60*60*24*30, //30天过期
						Issuer:    "imooc",
					},
				}
				token, err := j.CreateToken(claims)
				if err != nil {
					ctx.JSON(http.StatusInternalServerError, gin.H{
						"msg": "生成token失败",
					})
					return
				}

				ctx.JSON(http.StatusOK, gin.H{
					"id":         rsp.Id,
					"nick_name":  rsp.NickName,
					"token":      token,
					"expired_at": (time.Now().Unix() + 60*60*24*30) * 1000,
				})
			} else {
				ctx.JSON(http.StatusBadRequest, map[string]string{
					"msg": "登录失败",
				})
			}
		}
	}
}

func Register(ctx *gin.Context) {
	registerForm := forms.RegisterForm{}
	if err := ctx.ShouldBind(&registerForm); err != nil {
		HandleValidatorError(ctx, err)
		return
	}

	//验证码
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", global.ServerConfig.RedisInfo.Host, global.ServerConfig.RedisInfo.Port),
	})
	value, err := rdb.Get(context.Background(), registerForm.Mobile).Result()
	if err == redis.Nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code": "验证码错误",
		})
		return
	} else {
		if value != registerForm.Code {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"code": "验证码错误",
			})
			return
		}
	}

	userConn, err := grpc.NewClient(fmt.Sprintf("%s:%d", global.ServerConfig.UserSrvInfo.Host, global.ServerConfig.UserSrvInfo.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		zap.S().Errorw("[PassWordLogin] 连接用户服务失败",
			"msg", err.Error(),
		)
	}
	userSrvClient := proto.NewUserClient(userConn)
	user, err := userSrvClient.CreateUser(context.Background(), &proto.CreateUserInfo{
		NickName: registerForm.Mobile,
		PassWord: registerForm.PassWord,
		Mobile:   registerForm.Mobile,
	})

	if err != nil {
		zap.S().Errorf("[Register] 查询 【新建用户失败】失败: %s", err.Error())
		HandleGrpcErrorToHttp(err, ctx)
		return
	}

	j := middlewares.NewJWT()
	claims := models.CustomClaims{
		ID:          uint(user.Id),
		NickName:    user.NickName,
		AuthorityId: uint(user.Role),
		StandardClaims: jwt.StandardClaims{
			NotBefore: time.Now().Unix(),               //签名的生效时间
			ExpiresAt: time.Now().Unix() + 60*60*24*30, //30天过期
			Issuer:    "Orlando",
		},
	}
	token, err := j.CreateToken(claims)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"msg": "生成token失败",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"id":         user.Id,
		"nick_name":  user.NickName,
		"token":      token,
		"expired_at": (time.Now().Unix() + 60*60*24*30) * 1000,
	})
}
