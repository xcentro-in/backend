package business

// BusinessRepository defines the interface for interacting with business data.
type BusinessRepository interface {
	FindByID(id string) (*Business, error)
	Create(business *Business) error
	Update(business *Business) error
}

// BusinessUserRelationRepository defines the interface for the relationship data.
type BusinessUserRelationRepository interface {
	AddUserToBusiness(relation *BusinessUserRelation) error
	RemoveUserFromBusiness(userID string, bid string) error
	FindByUser(userID string) ([]*BusinessUserRelation, error)
	FindByBusiness(bid string) ([]*BusinessUserRelation, error)
}
