package services
 
import (
    "github.com/joho/godotenv"
    "cco_backend/config"
    "cco_backend/models"
    "cco_backend/utils"
    "fmt"
    "log"
    "strconv"
    "time"
    "os"
)
 
func ImportSkuData() error {
    err := godotenv.Load()
    if err != nil {
        return fmt.Errorf("error loading .env file: %w", err)
    }
 
    // Get the subscription ID from the environment variable
    subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
    if subscriptionID == "" {
        return fmt.Errorf("subscription ID not found in environment variables")
    }
 
    // Price API URL
    priceApiUrl := "https://prices.azure.com/api/retail/prices?api-version=2023-01-01-preview&$filter=serviceName%20eq%20%27Virtual%20Machines%27"
 
    // SKU API URL (Bearer token is required to access this API)
    skuApiUrl := fmt.Sprintf(
        "https://management.azure.com/subscriptions/%s/providers/Microsoft.Compute/skus?api-version=2024-07-01",
        subscriptionID,
    )
 
    // Fetch bearer token
    bearerToken, err := utils.GenerateBearerToken()
    if err != nil {
        return fmt.Errorf("error generating bearer token: %w", err)
    }
 
    // Fetch SKU data
    skuData, err := utils.FetchDataWithBearerToken(skuApiUrl, bearerToken)
    if err != nil {
        return fmt.Errorf("error fetching SKU data: %w", err)
    }
 
    skuItems, ok := skuData["value"].([]interface{}) // Extract the value from SKU data
    if !ok {
        return fmt.Errorf("invalid format for SKU items")
    }
 
    // Pagination loop for price data
    nextPageUrl := priceApiUrl
    pageCount := 0
    batchSize := 10 // Number of pages to process in each batch
 
    for nextPageUrl != "" {
        // Fetch price data for the current page
        priceData, err := utils.FetchData(nextPageUrl)
        if err != nil {
            return fmt.Errorf("error fetching price data: %w", err)
        }
 
        // Parse price items
        priceItems, ok := priceData["Items"].([]interface{})
        if !ok {
            return fmt.Errorf("invalid format for price items")
        }
 
        for _, priceItemInterface := range priceItems {
            priceItem, ok := priceItemInterface.(map[string]interface{})
            if !ok {
                log.Printf("Skipping invalid price item: %v", priceItemInterface)
                continue
            }
 
            // Extract required fields safely from price API
            skuID, _ := safeString(priceItem["skuId"])
            skuName, _ := safeString(priceItem["skuName"])
            productName, _ := safeString(priceItem["productName"])
            serviceFamily, _ := safeString(priceItem["serviceFamily"])
            armSkuName, ok := safeString(priceItem["armSkuName"])
            if !ok {
                log.Printf("Missing or invalid armSkuName: %v", priceItem)
                continue
            }
            skuType, ok := safeString(priceItem["type"])
            if !ok {
                log.Printf("Missing or invalid type: %v", priceItem)
                continue
            }
            regionName, _ := safeString(priceItem["armRegionName"])
 
            // Match with SKU API data
            var matchedSku map[string]interface{}
            for _, skuItemInterface := range skuItems {
                skuItem, ok := skuItemInterface.(map[string]interface{})
                if !ok {
                    continue
                }
                name, _ := safeString(skuItem["name"])
                if name == armSkuName {
                    matchedSku = skuItem
                    break
                }
            }
 
            if matchedSku == nil {
                log.Printf("No matching SKU found for armSkuName: %s", armSkuName)
                continue
            }
 
            // Extract details from matched SKU
            name, _ := safeString(matchedSku["name"])
            size, _ := safeString(matchedSku["size"])
 
            // Initialize variables
            var vCPUs int
            var memoryGB, cpuArchitectureType, maxNetworkInterfaces string
 
            capabilities, ok := matchedSku["capabilities"].([]interface{})
            if ok {
                for _, capabilityInterface := range capabilities {
                    capability, ok := capabilityInterface.(map[string]interface{})
                    if !ok {
                        continue
                    }
                    switch capName, _ := safeString(capability["name"]); capName {
                    case "vCPUs":
                        vCPUs = atoi(capability["value"].(string))
                    case "MemoryGB":
                        memoryGB, _ = safeString(capability["value"])
                    case "CpuArchitectureType":
                        cpuArchitectureType, _ = safeString(capability["value"])
                    case "MaxNetworkInterfaces":
                        maxNetworkInterfaces, _ = safeString(capability["value"])
                    }
                }
            }
 
            // Fetch service and region IDs
            service := models.Service{}
            region := models.Region{}
 
            if err := config.DB.Where("service_name = ?", "Virtual Machines").First(&service).Error; err != nil {
                log.Printf("Error finding service: %v", err)
                continue
            }
 
            if err := config.DB.Where("region_name = ?", regionName).First(&region).Error; err != nil {
                log.Printf("Error finding region: %v", err)
                continue
            }
 
            // Insert SKU into the database
            sku := models.Sku{
                ServiceID:            service.ServiceID,
                RegionID:             region.RegionID,
                Armskuname:           armSkuName,
                Name:                 name,
                Type:                 skuType,
                SkuIDAPI:             &skuID,
                SkuName:              &skuName,
                ProductName:          &productName,
                ServiceFamily:        &serviceFamily,
                InstanceSku:          nil,
                Size:                 size,
                VCPUs:                vCPUs,
                MemoryGB:             memoryGB,
                CpuArchitectureType:  cpuArchitectureType,
                OperatingSystem:      nil,
                MaxNetworkInterfaces: maxNetworkInterfaces,
                Storage:              nil,
            }
 
            result := config.DB.Create(&sku)
            if result.Error != nil {
                log.Printf("Error inserting SKU: %v", result.Error)
            } else {
                log.Printf("SKU inserted successfully: %v", sku.Name)
            }
        }
 
        // Increment page counter
        pageCount++
        if pageCount >= batchSize {
            log.Printf("Processed %d pages in this batch. Fetching next batch...", batchSize)
 
            // Reset the page count for the next batch
            pageCount = 0
 
            // Optional pause between batches
            time.Sleep(2 * time.Second)
        }
 
        // Check for the next page
        nextPageUrl, _ = safeString(priceData["NextPageLink"])
        log.Printf("Next page URL: %s", nextPageUrl)
    }
 
    log.Println("SKU data import completed successfully.")
    return nil
}
 
// Helper function to safely retrieve a string value from an interface{}
func safeString(value interface{}) (string, bool) {
    str, ok := value.(string)
    return str, ok
}
 
// Helper function to convert string to integer
func atoi(str string) int {
    val, err := strconv.Atoi(str)
    if err != nil {
        return 0
    }
    return val
}