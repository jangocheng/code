package cache

import (
	"github.com/TeaWeb/code/teaweb/helpers"
	"github.com/iwind/TeaGo"
)

func init() {
	TeaGo.BeforeStart(func(server *TeaGo.Server) {
		server.
			Prefix("/cache").
			Helper(new(helpers.UserMustAuth)).
			Helper(new(Helper)).
			Get("", new(IndexAction)).
			GetPost("/createPolicy", new(CreatePolicyAction)).
			Post("/deletePolicy", new(DeletePolicyAction)).
			GetPost("/updatePolicy", new(UpdatePolicyAction)).
			EndAll()
	})
}
