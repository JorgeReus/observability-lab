package controllers

import (
	msgbroker "banking-gateway/application/msg-broker"

	"github.com/gofiber/fiber/v2"
)

// Ready godoc
// @Summary Readiness probe
// @Description Readiness of the jwk service
// @ID readiness
// @Success 200
// @Router /readiness [get]
func ReadinessProbe(c *fiber.Ctx) error {
	if msgbroker.IsInitialized {
		return c.SendStatus(200)
	}
	return c.SendStatus(500)
}

// Healthy godoc
// @Summary Healhiness probe
// @Description Healthiness of the jwk service
// @ID healthz
// @Success 200
// @Router /healthz [get]
func Healthz(c *fiber.Ctx) error {
	if msgbroker.IsHealthy {
		return c.SendStatus(200)
	}
	return c.SendStatus(500)
}
