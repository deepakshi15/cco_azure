package services

import (
	"fmt"
	"log"
	"cco_backend/config"
	"cco_backend/models"
	"cco_backend/utils"
)

func ImportData() error { //fetch and import price data api
	baseURL := "https://prices.azure.com/api/retail/prices?api-version=2023-01-01-preview&$filter=serviceName%20eq%20%27Virtual%20Machines%27" 
	nextPageLink := baseURL

	for nextPageLink != "" { //loops through api's paginated responses until there are no more pages
		// Fetch data from the current page of the price API
		priceData, err := utils.FetchData(nextPageLink)
		if err != nil {
			return fmt.Errorf("error fetching price data: %w", err)
		}

		// Extract items from the JSON response-contains array of pricing data
		items, ok := priceData["Items"].([]interface{})
		if !ok {
			return fmt.Errorf("invalid data structure for items")
		}

		//iterates each item in the current page
		for _, item := range items {
			data := item.(map[string]interface{})
			//for region table
			regionName, _ := data["armRegionName"].(string)
			regionCode, _ := data["location"].(string)
			serviceName := "Virtual Machines"

			// Insert Provider if not exists
			provider := models.Provider{ProviderName: "Azure"}
			result := config.DB.Where("provider_name = ?", provider.ProviderName).FirstOrCreate(&provider)
			if result.Error != nil {
				log.Printf("Error inserting provider: %v", result.Error)
			} else {
				log.Printf("Provider inserted or already exists: %v", provider.ProviderName)
			}

			// Insert Region if not exists
			region := models.Region{
				ProviderID: provider.ProviderID,
				RegionCode: regionCode,
				RegionName: regionName,
			}
			result = config.DB.Where("region_code = ?", region.RegionCode).FirstOrCreate(&region)
			if result.Error != nil {
				log.Printf("Error inserting region: %v", result.Error)
			} else {
				log.Printf("Region inserted or already exists: %v", region.RegionCode)
			}

			// Insert Service if not exists
			service := models.Service{
				ProviderID:  provider.ProviderID,
				ServiceName: serviceName,
			}
			result = config.DB.Where("service_name = ?", service.ServiceName).FirstOrCreate(&service)
			if result.Error != nil {
				log.Printf("Error inserting service: %v", result.Error)
			} else {
				log.Printf("Service inserted or already exists: %v", service.ServiceName)
			}
		}

		// Update the nextPageLink for the next iteration-handles pagination
		if nextLink, ok := priceData["NextPageLink"].(string); ok && nextLink != "" {
			nextPageLink = nextLink
		} else {
			nextPageLink = "" // Exit the loop if there's no next page
		}
	}

	fmt.Println("Data import completed successfully!")
	return nil
}