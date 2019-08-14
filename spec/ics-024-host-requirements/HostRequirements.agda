module HostRequirements where

open import Agda.Builtin.Unit
open import Data.String as String using (String)
open import Category.Applicative
open import Category.Monad
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

{-
get : Key -> State Env Value
get key = return (MkValue "")

set : Key -> Value -> State Env ⊤
set key value = return ⊤

delete : Key -> State Env ⊤
delete key = return ⊤

getConsensusState : State Env ConsensusState
getConsensusState = ?

getCallingModule : State Env Identifier
getCallingModule = return (MkIdentifier "")
-}
