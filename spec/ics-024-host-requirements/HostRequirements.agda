module HostRequirements where

open import Agda.Builtin.Unit
open import Data.String as String using (String)
open import Category.Monad.State

data Key : Set where
  MkKey : String -> Key

data Value : Set where
  MkValue : String -> Value

data Identifier : Set where
  MkIdentifier : String -> Identifier

separator : String
separator = "/"

data Env : Set where
  MkEnv : Env

get : Key -> State Env Value
get key = ?

set : Key -> Value -> State Env ⊤
set key value = ?

delete : Key -> State Env ⊤
delete key = ?

{-
getConsensusState : State Env ConsensusState
getConsensusState = ?
-}

getCallingModule : State Env Identifier
getCallingModule = ?
