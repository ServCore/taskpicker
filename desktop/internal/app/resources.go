package app

import "taskpicker/internal/resources"

func (a *App) GetResourceUsage() (resources.ResourceUsage, error) { return a.resources.Sample() }
