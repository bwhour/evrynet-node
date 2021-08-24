package state

import "errors"

var (
	ErrMaxProvider   = errors.New("maximum providers in contract")
	ErrOnlyProvider  = errors.New("only provider can execute transaction to enterprise contract")
	ErrOnlyOwner     = errors.New("only owner can add or remove provider")
	ErrOwnerNotFound = errors.New("adding or removing provider transaction should be sent to enterprise contract")
)
