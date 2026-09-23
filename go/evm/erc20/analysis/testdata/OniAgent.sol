// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/*
           :::      ::::::::  :::::::::: ::::    ::: ::::::::::: ::::::::::: ::::::::  :::    ::: :::::::::: ::::    ::: :::     :::  ::::::::
       :+: :+:   :+:    :+: :+:        :+:+:   :+:     :+:         :+:    :+:    :+: :+:   :+:  :+:        :+:+:   :+: :+:     :+: :+:    :+:
     +:+   +:+  +:+        +:+        :+:+:+  +:+     +:+         +:+    +:+    +:+ +:+  +:+   +:+        :+:+:+  +:+ +:+     +:+        +:+
   +#++:++#++: :#:        +#++:++#   +#+ +:+ +#+     +#+         +#+    +#++:++   +#++:++    +#++:++#   %# ++ :# +#++:++#++     +#++:
  +#++     +#++#++   +#++#++#        +#++  +#++#     #++         #++    +#++  +#++ +#++  +#++   +#++        +#++#+#  +#++     +#++         +#++
 #++      +#++#++    #+# #+#        #++   #+#+#     #+#         #+#    #++   #+# #++   #+#  #+#        #++  #+#   #++#    #+#     #++      #+#
###      +########  ###  ###        ###    ####     ###         ###     ###### ###  ###    ### ########## ###    ####     ########        ###
*/

/// @title  OniAgent
/// @notice Optimized ERC20 + Metadata
contract OniAgent {
    /*`':°•.°+.*•´.*:˚.°*.˚•´.°:°•.°•.*•´.*:˚.°*.˚•´.°:°•.°+.*•´.*:*/
    /*                       CUSTOM ERRORS                        */
    /*.•°:°.´+˚.*°.˚:*.´•*.+°.•°:´*.´•*.•°.•°:°.´:•˚°.*°.˚:*.´+°.•*/

    error Unauthorized();
    error InsufficientBalance();
    error InsufficientAllowance();
    error TransferFromZeroAddress();
    error TransferToZeroAddress();

    /*`':°•.°+.*•´.*:˚.°*.˚•´.°:°•.°•.*•´.*:˚.°*.˚•´.°:°•.°+.*•´.*:*/
    /*                           EVENTS                           */
    /*.•°:°.´+˚.*°.˚:*.´•*.+°.•°:´*.´•*.•°.•°:°.´:•˚°.*°.˚:*.´+°.•*/

    event Transfer(address indexed from, address indexed to, uint256 amount);
    event Approval(address indexed owner, address indexed spender, uint256 amount);
    event MetadataSet(string imageUrl, string description, string website, string twitter);
    event OwnershipRenounced(address indexed previousOwner);

    /*`':°•.°+.*•´.*:˚.°*.˚•´.°:°•.°•.*•´.*:˚.°*.˚•´.°:°•.°+.*•´.*:*/
    /*                          STORAGE                           */
    /*.•°:°.´+˚.*°.˚:*.´•*.+°.•°:´*.´•*.•°.•°:°.´:•˚°.*°.˚:*.´+°.•*/

    string  private _name;
    string  private _symbol;
    uint8   private constant _DECIMALS = 18;

    uint256 private _totalSupply;
    mapping(address => uint256) private _balances;
    mapping(address => mapping(address => uint256)) private _allowances;

    address public owner;
    string  public imageUrl;
    string  public description;
    string  public website;
    string  public twitter;

    /*`':°•.°+.*•´.*:˚.°*.˚•´.°:°•.°•.*•´.*:˚.°*.˚•´.°:°•.°+.*•´.*:*/
    /*                        CONSTRUCTOR                         */
    /*.•°:°.´+˚.*°.˚:*.´•*.+°.•°:´*.´•*.•°.•°:°.´:•˚°.*°.˚:*.´+°.•*/

    constructor(string memory name_, string memory symbol_, uint256 supply_) {
        _name   = name_;
        _symbol = symbol_;
        owner   = msg.sender;

        uint256 amount = supply_ * 10 ** _DECIMALS;
        _totalSupply = amount;
        _balances[msg.sender] = amount;

        emit Transfer(address(0), msg.sender, amount);
    }

    /*`':°•.°+.*•´.*:˚.°*.˚•´.°:°•.°•.*•´.*:˚.°*.˚•´.°:°•.°+.*•´.*:*/
    /*                       ERC20 METADATA                       */
    /*.•°:°.´+˚.*°.˚:*.´•*.+°.•°:´*.´•*.•°.•°:°.´:•˚°.*°.˚:*.´+°.•*/

    function name() public view returns (string memory) {
        return _name;
    }

    function symbol() public view returns (string memory) {
        return _symbol;
    }

    function decimals() public pure returns (uint8) {
        return _DECIMALS;
    }

    function totalSupply() public view returns (uint256) {
        return _totalSupply;
    }

    function balanceOf(address account) public view returns (uint256) {
        return _balances[account];
    }

    /*`':°•.°+.*•´.*:˚.°*.˚•´.°:°•.°•.*•´.*:˚.°*.˚•´.°:°•.°+.*•´.*:*/
    /*                           ERC20                            */
    /*.•°:°.´+˚.*°.˚:*.´•*.+°.•°:´*.´•*.•°.•°:°.´:•˚°.*°.˚:*.´+°.•*/

    function transfer(address to, uint256 amount) public returns (bool) {
        _transfer(msg.sender, to, amount);
        return true;
    }

    function allowance(address owner_, address spender) public view returns (uint256) {
        return _allowances[owner_][spender];
    }

    function approve(address spender, uint256 amount) public returns (bool) {
        _approve(msg.sender, spender, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) public returns (bool) {
        uint256 allowed = _allowances[from][msg.sender];
        if (allowed != type(uint256).max) {
            if (allowed < amount) revert InsufficientAllowance();
            unchecked {
                _allowances[from][msg.sender] = allowed - amount;
            }
        }
        _transfer(from, to, amount);
        return true;
    }

    function _transfer(address from, address to, uint256 amount) internal {
        if (from == address(0)) revert TransferFromZeroAddress();
        if (to == address(0)) revert TransferToZeroAddress();

        uint256 fromBalance = _balances[from];
        if (fromBalance < amount) revert InsufficientBalance();

        unchecked {
            _balances[from] = fromBalance - amount;
            _balances[to] += amount;
        }

        emit Transfer(from, to, amount);
    }

    function _approve(address owner_, address spender, uint256 amount) internal {
        _allowances[owner_][spender] = amount;
        emit Approval(owner_, spender, amount);
    }

    /*`':°•.°+.*•´.*:˚.°*.˚•´.°:°•.°•.*•´.*:˚.°*.˚•´.°:°•.°+.*•´.*:*/
    /*                          METADATA                          */
    /*.•°:°.´+˚.*°.˚:*.´•*.+°.•°:´*.´•*.•°.•°:°.´:•˚°.*°.˚:*.´+°.•*/

    function setMetadata(
        string calldata _imageUrl,
        string calldata _description,
        string calldata _website,
        string calldata _twitter
    ) external {
        if (msg.sender != owner) revert Unauthorized();

        imageUrl    = _imageUrl;
        description = _description;
        website     = _website;
        twitter     = _twitter;

        emit MetadataSet(_imageUrl, _description, _website, _twitter);
    }

    function renounceOwnership() external {
        if (msg.sender != owner) revert Unauthorized();

        emit OwnershipRenounced(owner);
        owner = address(0);
    }

    function contractURI() external view returns (string memory) {
        return string(
            abi.encodePacked(
                '{"name":"',        _escapeJson(_name),
                '","symbol":"',     _escapeJson(_symbol),
                '","image":"',      imageUrl,
                '","description":"',_escapeJson(description),
                '","external_link":"', website,
                '","twitter":"',    twitter,
                '"}'
            )
        );
    }

    function _escapeJson(string memory s) internal pure returns (string memory) {
        bytes memory b = bytes(s);
        bytes memory out = new bytes(b.length * 2);
        uint256 len;

        for (uint256 i; i < b.length; ++i) {
            bytes1 c = b[i];
            if (c == '"' || c == '\\') {
                out[len++] = '\\';
            }
            out[len++] = c;
        }

        bytes memory result = new bytes(len);
        for (uint256 i; i < len; ++i) {
            result[i] = out[i];
        }
        return string(result);
    }
}
