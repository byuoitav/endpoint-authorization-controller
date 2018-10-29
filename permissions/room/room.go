package room

import (
	"fmt"
	"time"

	"github.com/byuoitav/common/nerr"
	"github.com/byuoitav/endpoint-authorization-controller/base"
	"github.com/byuoitav/endpoint-authorization-controller/db"
	"github.com/byuoitav/endpoint-authorization-controller/users"
)

//CalculateRoomPermissions given the request with the 'room' resource type - calculate the permissions allowed.
//We assume that the Access Key associated with the request has already been validated.
func CalculateRoomPermissions(req base.UserInformation) (base.Response, *nerr.E) {
	toReturn := base.Response{
		CommonInfo: req.CommonInfo,
	}

	//set of roles
	roles := map[string]bool{}

	if req.ResourceType != base.Room {
		return toReturn, nerr.Create(fmt.Sprintf("Invalid request type. Must be %v", base.Room), "invalid-type")
	}

	//we need to get the list of groups for the user
	groups, err := users.GetGroupsForUser(req.ID)
	if err != nil {
		return toReturn, err.Addf("Couldn't calcualte room permissions for %v", req.ResourceID)
	}

	//we need to get the list of permissions associated with the type and id
	records, err := db.GetPermissionRecords(req.ResourceType, req.ResourceID)
	if err != nil {
		return toReturn, err.Addf("Couldn't calcualte room permissions for %v", req.ResourceID)
	}

	curTTL := 0

	//start at the 'all' level and look at the permissions denoted there
	if v, ok := records["*"]; ok {

		for k, j := range v.Allow {
			if _, ok := groups[k]; ok {

				//set the TTL
				if j.TTL != nil {
					if curTTL == 0 || curTTL > *j.TTL {
						curTTL = *j.TTL
					}
				}

				//everything is allow at this point
				for _, l := range j.Roles {
					roles[l] = true
				}
			}
		}
	}

	toReturn, err = GetPermissionsForSubResources(records, "*", roles, groups, curTTL)
	if err != nil {
		return toReturn, err.Addf("Couldn't Build permission set for resource: %v and user %v", req.ResourceID, req.ID)
	}

	return toReturn, nil
}

//GetPermissionsForSubResources .
func GetPermissionsForSubResources(records map[string]base.PermissionsRecord, currentResource string, parentRoles, groups map[string]bool, curTTL int) (base.Response, *nerr.E) {
	toReturn := base.Response{
		Permissions: map[string][]string{},
	}

	roles := map[string]bool{}

	//inherit the allow permissions
	for k, v := range parentRoles {
		if v {
			roles[k] = true
		}
	}

	//build the roles from this level
	v, ok := records[currentResource]
	if ok {
		//do our Deny first, since we can reverse this later with the allows
		for k, j := range v.Deny {
			if _, ok := groups[k]; ok {

				//set the TTL
				if j.TTL != nil {
					if curTTL == 0 || curTTL > *j.TTL {
						curTTL = *j.TTL
					}
				}

				for _, l := range j.Roles {
					roles[l] = false
				}
			}
		}
		//allow permissions override
		for k, j := range v.Allow {
			if _, ok := groups[k]; ok {

				//set the TTL
				if j.TTL != nil {
					if curTTL == 0 || curTTL > *j.TTL {
						curTTL = *j.TTL
					}
				}

				for _, l := range j.Roles {
					roles[l] = true
				}
			}
		}
	}

	nowTTL := time.Now().Add(time.Duration(curTTL) * time.Second)

	//it's a leaf node
	if !ok || len(records[currentResource].SubResources) < 1 {
		for k, v := range roles {
			if v {
				toReturn.Permissions[currentResource] = append(toReturn.Permissions[currentResource], k)
			}
			toReturn.TTL = nowTTL
		}
		return toReturn, nil
	}

	//recurse
	for _, v := range records[currentResource].SubResources {

		//get their stuff, aggregate it
		resp, err := GetPermissionsForSubResources(records, v, roles, groups, curTTL)
		if err != nil {
			return toReturn, err.Addf("Coudn't generate permissions for %v and subresources", currentResource)
		}

		//Check for a shorter TTL
		if nowTTL.After(resp.TTL) {
			nowTTL = resp.TTL
		}

		for k, v := range resp.Permissions {
			toReturn.Permissions[k] = v
		}
	}
	return toReturn, nil
}