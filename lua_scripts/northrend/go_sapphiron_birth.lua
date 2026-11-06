-- go_sapphiron_birth (Naxxramas) — Lua port of the GameObjectAI in
-- src/server/scripts/Northrend/Naxxramas/boss_sapphiron.cpp
-- (go_sapphiron_birthAI, :464-490): on GO_ACTIVATED the birth GO
-- cross-AIs Sapphiron DoAction(ACTION_BIRTH 1) via the instance
-- GetGuidData(DATA_SAPPHIRON) handle and sets instance
-- SetData(DATA_HAD_SAPPHIRON_BIRTH, 1); on GO_JUST_DEACTIVATED it
-- sets respawn 0 and deletes itself.
-- Go twin: the player-use path fires GameObjectEventOnUse (14),
-- not OnLootState (9 — only Lua SetLootState fires it), so the
-- ACTIVATED leg rides OnUse: the birth GO is a use-activated GO,
-- and the use IS the activation in C++ (GameObject::Use ->
-- SetLootState(GO_ACTIVATED)). The JUST_DEACTIVATED delete leg has
-- no fire site (no autoCloseTime model), so the GO despawns at
-- once (hidden for the instance lifetime, the Delete +
-- SetRespawnTime(0) twin) and the had-birth latch makes repeat
-- uses no-ops. The instance SetData leg rides the shared
-- instanceData table (instance_naxxramas.lua documents the keys);
-- the OnGameObjectCreate birth-delete leg (had-birth or Sapphiron
-- DONE at create time) is the sapphironDone guard below — static
-- GOs never fire create in Go.
-- Entry 181356 (naxxramas.h GO_BIRTH :162); Sapphiron 15990 is
-- bound by instance_naxxramas.lua ON_CREATURE_CREATE.

local GO_BIRTH = 181356
local ACTION_BIRTH = 1

local function sapphironBirthUse(event, go, player)
    if player == nil then
        return
    end
    local mapObj = go:GetMap()
    if mapObj == nil then
        return
    end
    local data = mapObj:GetInstanceData()
    if data == nil then
        return
    end
    if data.hadSapphironBirth then
        return
    end
    if data.sapphironDone then
        go:Despawn()
        return
    end
    local sapphiron = data.sapphiron
    if sapphiron ~= nil then
        sapphiron:DoAction(ACTION_BIRTH)
    end
    data.hadSapphironBirth = true
    go:Despawn()
end

RegisterGameObjectEvent(GO_BIRTH, 14, sapphironBirthUse)
