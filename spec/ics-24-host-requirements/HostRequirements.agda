module HostRequirements where

open import Data.String as String using (String)
open import Data.AVL.IndexedMap as Map using (Map)

data Key : Set where
  MkKey : String -> Key

data Value : Set where
  MkValue : String -> Value

data Identifier : Set where
  MkIdentifier : String -> Identifier

separator : String
separator = "/"

data Env : Set where
  MkEnv : Map.Map -> Env

get : Key -> State Env Value
get key = undefined

set : Key -> Value -> State Env ()
set key value = undefined

delete : Key -> State Env ()
delete key = undefined

getConsensusState : State Env ConsensusState
getConsensusState = undefined

getCallingModule : State Env Identifier
getCallingModule = undefined
