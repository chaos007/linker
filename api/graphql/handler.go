package graphql

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
	"github.com/wpajqz/linker/client"
	"github.com/wpajqz/linker/codec"
)

var queryType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Query",
	Fields: graphql.Fields{
		"request": &graphql.Field{
			Name: "Request",
			Type: graphql.String,
			Args: graphql.FieldConfigArgument{
				"method": &graphql.ArgumentConfig{
					Type: graphql.String,
				},
				"param": &graphql.ArgumentConfig{Type: graphql.String},
			},
		},
	},
})

func (ja *graphqlAPI) hf(h *handler.Handler) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var requestType = h.Schema.QueryType().Fields()["request"]
		if requestType != nil {
			requestType.Resolve = func(p graphql.ResolveParams) (i interface{}, err error) {
				method := p.Args["method"]
				param := p.Args["param"]

				session, err := brpc.Session()
				if err != nil {
					return nil, err
				}

				var (
					b           []byte
					errCallback error
				)

				for k, v := range ctx.Request.Header {
					session.SetRequestProperty(k, strings.Join(v, ","))
				}

				coder, err := codec.NewCoder(session.GetContentType())
				if err != nil {
					return nil, err
				}

				var body map[string]interface{}
				if param != nil {
					err := coder.Decoder([]byte(param.(string)), &body)
					if err != nil {
						return nil, err
					}
				}

				to, _ := context.WithTimeout(context.Background(), ja.options.timeout)
				err = session.SyncSendWithTimeout(to, method.(string), body, client.RequestStatusCallback{
					Success: func(header, body []byte) {
						for _, v := range strings.Split(string(header), ";") {
							if len(v) > 0 {
								ss := strings.Split(v, "=")
								if len(ss) > 1 {
									ctx.Writer.Header().Set(ss[0], ss[1])
								}
							}
						}

						b = body
					},
					Error: func(code int, message string) {
						errCallback = errors.New(message)
					},
				})

				if err != nil {
					return nil, err
				}

				if errCallback != nil {
					return nil, errCallback
				}

				return string(b), nil
			}
		}

		h.ServeHTTP(ctx.Writer, ctx.Request)
	}
}
