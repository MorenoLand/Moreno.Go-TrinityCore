-- Sholazar Basin zone spell scripts -- Lua ports of the SpellScript
-- arms in src/server/scripts/Northrend/zone_sholazar_basin.cpp.
--
-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_q12611_deathbolt::HandleScriptEffect
--   (zone_sholazar_basin.cpp:658-689): EFFECT_0 SCRIPT_EFFECT. The
--   hit unit carrying Freya's Ward (51845) casts Sevenfold
--   Retribution (51856) on the caster, triggered; otherwise the
--   caster casts Deathbolt (51855) on the hit unit, triggered.
--   Binding 51854 (AC 'spell_q12611_deathbolt'; TDB 335.26091
--   renames it 'spell_sholazar_deathbolt').
local SPELL_DEATHBOLT = 51854 -- zone_sholazar_basin.cpp:658
local SPELL_FREYAS_WARD = 51845 -- zone_sholazar_basin.cpp:654
local SPELL_SEVENFOLD_RETRIBUTION = 51856 -- zone_sholazar_basin.cpp:656
local SPELL_DEATHBOLT_TRIGGER = 51855 -- zone_sholazar_basin.cpp:657

local function deathboltHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if hitTarget:HasAura(SPELL_FREYAS_WARD) then
        hitTarget:CastSpell(caster, SPELL_SEVENFOLD_RETRIBUTION, true)
    else
        caster:CastSpell(hitTarget, SPELL_DEATHBOLT_TRIGGER, true)
    end
end

RegisterSpellEvent(SPELL_DEATHBOLT, 5, deathboltHit)

local SPELL_SHOOT_RJR = 51330 -- zone_sholazar_basin.cpp:460 (class spell_q12589_shoot_rjr)
local NPC_LUCKY_WILHELM = 28054 -- zone_sholazar_basin.cpp:440
local SPELL_FAILED_CUSTOM_ERROR = 172 -- SharedDefines.h:1154
local SPELL_CUSTOM_ERROR_MUST_TARGET_WILHELM = 27 -- SharedDefines.h:1203
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

-- C++ spell_q12589_shoot_rjr::CheckCast (zone_sholazar_basin.cpp:470-478):
-- the explicit target must be Lucky Wilhelm (28054) or the cast fails
-- SPELL_FAILED_CUSTOM_ERROR with MUST_TARGET_WILHELM. RegisterSpellEvent
-- OnCheckCast = 8 (lua_spell.go fireSpellCheckCast); the second return
-- is the custom-error id the fire site sends with the failure. Binding
-- 51330 is the AC spell_script_names row. The HandleDummy roll arm
-- (EVENT_MISS/HIT/MISS_BIRD) is PORTED below (shootRjrDummy, ev5).
local function shootRjrCheckCast(event, spell)
    local target = spell:GetTarget()
    if target ~= nil and target:GetEntry() == NPC_LUCKY_WILHELM then
        return SPELL_CAST_OK
    end
    return SPELL_FAILED_CUSTOM_ERROR, SPELL_CUSTOM_ERROR_MUST_TARGET_WILHELM
end

RegisterSpellEvent(SPELL_SHOOT_RJR, 8, shootRjrCheckCast)

-- C++ spell_q12589_shoot_rjr::HandleDummy (zone_sholazar_basin.cpp:480-536):
-- OnEffectHitTarget EFFECT_0 SPELL_EFFECT_DUMMY. urand(1,100): <=50 MISS,
-- <=83 HIT, else MISS_BIRD. MISS: shooter casts 51332 on Wilhelm, Wilhelm
-- Talk(0), Drostan Talk(0). HIT: shooter casts 51331 on the apple, apple
-- self-casts 51371, Wilhelm Talk(1), player KilledMonsterCredit(28053).
-- MISS_BIRD: shooter casts 51366 on Thickbird, bird self-casts 51369,
-- Wilhelm + Drostan Talk(0), bird KillSelf, Crunchy MovePoint(0, bird
-- X/Y, water-or-ground level) — and if bird or crunchy is missing, falls
-- through to the MISS leg ([[fallthrough]], :523-524). Returns silently
-- if Wilhelm, apple, or Drostan is missing (:498-499). All C++ casts are
-- non-triggered (no `true` arg). DORMANT until Spell.dbc ships 51330.
local NPC_APPLE = 28053 -- zone_sholazar_basin.cpp:441
local NPC_DROSTAN = 28328 -- zone_sholazar_basin.cpp:442
local NPC_CRUNCHY = 28346 -- zone_sholazar_basin.cpp:443
local NPC_THICKBIRD = 28093 -- zone_sholazar_basin.cpp:444
local SPELL_HIT_APPLE = 51331 -- zone_sholazar_basin.cpp:447
local SPELL_MISS_APPLE = 51332 -- zone_sholazar_basin.cpp:448
local SPELL_MISS_BIRD_APPLE = 51366 -- zone_sholazar_basin.cpp:449
local SPELL_APPLE_FALL = 51371 -- zone_sholazar_basin.cpp:450
local SPELL_BIRD_FALL = 51369 -- zone_sholazar_basin.cpp:451
local SAY_WILHELM_MISS = 0 -- zone_sholazar_basin.cpp:457
local SAY_WILHELM_HIT = 1 -- zone_sholazar_basin.cpp:458
local SAY_DROSTAN_REPLY_MISS = 0 -- zone_sholazar_basin.cpp:459

local function shootRjrDummy(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local shooter = spell:GetCaster()
    if shooter == nil or hitTarget == nil then
        return
    end
    -- CheckCast guarantees the hit unit is Lucky Wilhelm (28054);
    -- the Creature gate is the C++ GetHitUnit()->ToCreature() twin.
    local wilhelm = hitTarget
    if wilhelm:GetObjectType() ~= "Creature" then
        return
    end
    local apple = shooter:FindNearestCreature(NPC_APPLE, 30.0)
    local drostan = shooter:FindNearestCreature(NPC_DROSTAN, 30.0)
    if apple == nil or drostan == nil then
        return
    end
    local roll = math.random(1, 100)
    if roll <= 50 then
        -- EVENT_MISS
        shooter:CastSpell(wilhelm, SPELL_MISS_APPLE)
        wilhelm:Talk(SAY_WILHELM_MISS)
        drostan:Talk(SAY_DROSTAN_REPLY_MISS)
    elseif roll <= 83 then
        -- EVENT_HIT
        shooter:CastSpell(apple, SPELL_HIT_APPLE)
        apple:CastSpell(apple, SPELL_APPLE_FALL)
        wilhelm:Talk(SAY_WILHELM_HIT)
        local player = shooter:ToPlayer()
        if player ~= nil then
            player:KilledMonsterCredit(NPC_APPLE)
        end
    else
        -- EVENT_MISS_BIRD; falls through to the MISS leg if bird or
        -- crunchy is missing (C++ [[fallthrough]], :523-524).
        local bird = shooter:FindNearestCreature(NPC_THICKBIRD, 30.0)
        local crunchy = shooter:FindNearestCreature(NPC_CRUNCHY, 30.0)
        if bird ~= nil and crunchy ~= nil then
            -- Bird position is read before Kill() so the MovePoint
            -- target survives the kill.
            local bx, by, bz = bird:GetX(), bird:GetY(), bird:GetZ()
            shooter:CastSpell(bird, SPELL_MISS_BIRD_APPLE)
            bird:CastSpell(bird, SPELL_BIRD_FALL)
            wilhelm:Talk(SAY_WILHELM_MISS)
            drostan:Talk(SAY_DROSTAN_REPLY_MISS)
            bird:Kill()
            -- C++ passes GetWaterOrGroundLevel(...) as the MovePoint Z;
            -- no bridge exists (zero hits in engine/), so bird:GetZ()
            -- is used instead.
            crunchy:MoveTo(0, bx, by, bz)
        else
            shooter:CastSpell(wilhelm, SPELL_MISS_APPLE)
            wilhelm:Talk(SAY_WILHELM_MISS)
            drostan:Talk(SAY_DROSTAN_REPLY_MISS)
        end
    end
end

RegisterSpellEvent(SPELL_SHOOT_RJR, 5, shootRjrDummy)

-- C++ spell_shango_tracks::HandleScript (zone_sholazar_basin.cpp:622-636):
-- OnEffectHitTarget EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT; the hit unit
-- Says the correct/incorrect tracks text by spell id. No cast site of
-- 52160/52163 exists in the C++ tree, so the hit unit is assumed to be
-- a creature — if it is the player, the Say leg has no text-id bridge
-- (player Say takes a string) and the handler returns silently. The
-- C++ Say target arg is range/audience, not a whisper, so no whisper
-- target is passed. DORMANT until Spell.dbc ships 52160/52163.
local SPELL_CORRECT_TRACKS = 52160 -- zone_sholazar_basin.cpp:606
local SPELL_INCORRECT_TRACKS = 52163 -- zone_sholazar_basin.cpp:607
local SAY_CORRECT_TRACKS = 28634 -- zone_sholazar_basin.cpp:608
local SAY_INCORRECT_TRACKS = 28635 -- zone_sholazar_basin.cpp:609

local function shangoTracks(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    local id = spell:GetEntry()
    if id == SPELL_CORRECT_TRACKS then
        hitTarget:Talk(SAY_CORRECT_TRACKS)
    elseif id == SPELL_INCORRECT_TRACKS then
        hitTarget:Talk(SAY_INCORRECT_TRACKS)
    end
end

RegisterSpellEvent(SPELL_CORRECT_TRACKS, 5, shangoTracks)
RegisterSpellEvent(SPELL_INCORRECT_TRACKS, 5, shangoTracks)
